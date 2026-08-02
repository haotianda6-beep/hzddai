from __future__ import annotations

from datetime import datetime, timezone
from decimal import Decimal

from sqlalchemy import func, select
from sqlalchemy.orm import Session

from app.amounts import money, rate
from app.models import (
    ROLE_RATE,
    CommissionLedger,
    CommissionWalletLedger,
    DepositEvent,
    User,
)
from app.qualification import revoke_ib_qualification, update_ib_qualification


def utcnow() -> datetime:
    return datetime.now(timezone.utc)


def ancestor_chain(session: Session, user: User) -> list[User]:
    chain: list[User] = []
    seen = {user.id}
    parent_id = user.parent_id
    while parent_id:
        if parent_id in seen:
            raise ValueError("邀请关系存在循环")
        parent = session.get(User, parent_id)
        if not parent:
            break
        chain.append(parent)
        seen.add(parent.id)
        parent_id = parent.parent_id
    return chain


def is_descendant(session: Session, ancestor_id: int, user: User) -> bool:
    return any(row.id == ancestor_id for row in ancestor_chain(session, user))


def wallet_delta(
    session: Session,
    user: User,
    delta: Decimal,
    reason: str,
    reference_type: str,
    reference_id: int,
) -> None:
    user.rebate_balance_usdt = money(user.rebate_balance_usdt + delta)
    session.add(
        CommissionWalletLedger(
            user_id=user.id,
            delta_usdt=money(delta),
            available_after_usdt=user.rebate_balance_usdt,
            frozen_after_usdt=money(user.frozen_balance_usdt),
            reason=reason,
            reference_type=reference_type,
            reference_id=reference_id,
        )
    )


def _allocate_commission(session: Session, event: DepositEvent, depositor: User) -> list[CommissionLedger]:
    absorbed = Decimal("0")
    allocated = Decimal("0")
    rows: list[CommissionLedger] = []
    for depth, ancestor in enumerate(ancestor_chain(session, depositor), start=1):
        cap = ROLE_RATE.get(ancestor.role, Decimal("0"))
        if cap <= absorbed:
            continue
        delta_rate = cap - absorbed
        target = money(event.amount_usdt * cap / Decimal("100"))
        amount = money(target - allocated)
        absorbed = cap
        if amount <= 0:
            continue
        allocated = money(allocated + amount)
        row = CommissionLedger(
            deposit_event_id=event.id,
            source_user_id=depositor.id,
            recipient_user_id=ancestor.id,
            amount_usdt=amount,
            recipient_role=ancestor.role,
            rate_percent=delta_rate,
            path_depth=depth,
            entry_type="commission",
        )
        session.add(row)
        session.flush()
        wallet_delta(session, ancestor, amount, "commission", "commission", row.id)
        rows.append(row)
    return rows


def validate_deposit_replay(event: DepositEvent, user: User, amount: Decimal) -> None:
    if event.event_type != "deposit" or event.user_id != user.id or money(event.amount_usdt) != amount:
        raise ValueError("external_ref流水号冲突")


def validate_reversal_replay(event: DepositEvent, original: DepositEvent, amount: Decimal) -> None:
    if (
        event.event_type != "reversal"
        or event.original_event_id != original.id
        or money(-event.amount_usdt) != amount
    ):
        raise ValueError("external_ref流水号冲突")


def record_deposit(
    session: Session,
    user: User,
    amount: Decimal,
    external_ref: str,
    source: str,
    note: str | None = None,
    occurred_at: datetime | None = None,
) -> tuple[DepositEvent, bool]:
    external_ref = external_ref.strip()
    amount = money(amount)
    if amount <= 0:
        raise ValueError("确认充值金额必须大于0")
    if not external_ref:
        raise ValueError("external_ref不能为空")
    existing = session.scalar(select(DepositEvent).where(DepositEvent.external_ref == external_ref))
    if existing:
        validate_deposit_replay(existing, user, amount)
        return existing, True
    when = occurred_at or utcnow()
    event = DepositEvent(
        external_ref=external_ref,
        user_id=user.id,
        amount_usdt=amount,
        event_type="deposit",
        source=source.strip() or "confirmed_deposit",
        note=(note or "").strip() or None,
        occurred_at=when,
    )
    session.add(event)
    session.flush()
    user.lifetime_deposit_usdt = money(user.lifetime_deposit_usdt + amount)
    _allocate_commission(session, event, user)
    # 第5名达标用户的本笔充值完成分佣后才升级，因此门槛充值不向新IB补发。
    update_ib_qualification(session, user, when)
    return event, False


def reverse_deposit(
    session: Session,
    original: DepositEvent,
    amount: Decimal,
    external_ref: str,
    note: str | None = None,
) -> tuple[DepositEvent, bool]:
    external_ref = external_ref.strip()
    amount = money(amount)
    if original.event_type != "deposit" or money(original.amount_usdt) <= 0 or amount <= 0:
        raise ValueError("冲正参数无效")
    if not external_ref:
        raise ValueError("external_ref不能为空")
    existing = session.scalar(select(DepositEvent).where(DepositEvent.external_ref == external_ref))
    if existing:
        validate_reversal_replay(existing, original, amount)
        return existing, True
    reversed_total = session.scalar(
        select(func.coalesce(func.sum(-DepositEvent.amount_usdt), 0)).where(
            DepositEvent.original_event_id == original.id,
            DepositEvent.event_type == "reversal",
        )
    )
    reversed_before = money(reversed_total or 0)
    original_amount = money(original.amount_usdt)
    if amount > money(original_amount - reversed_before):
        raise ValueError("冲正金额超过原确认充值可冲正余额")
    event = DepositEvent(
        external_ref=external_ref,
        user_id=original.user_id,
        amount_usdt=-amount,
        event_type="reversal",
        original_event_id=original.id,
        source="deposit_reversal",
        note=(note or "").strip() or None,
        occurred_at=utcnow(),
    )
    session.add(event)
    session.flush()
    depositor = session.get(User, original.user_id) if original.user_id else None
    if depositor:
        depositor.lifetime_deposit_usdt = money(
            max(Decimal("0"), depositor.lifetime_deposit_usdt - amount)
        )
        revoke_ib_qualification(session, depositor, event.occurred_at)
    original_rows = session.scalars(
        select(CommissionLedger).where(
            CommissionLedger.deposit_event_id == original.id,
            CommissionLedger.entry_type == "commission",
        )
    ).all()
    cumulative_reversal = money(reversed_before + amount)
    for old in original_rows:
        already_reversed = money(
            session.scalar(
                select(func.coalesce(func.sum(-CommissionLedger.amount_usdt), 0)).where(
                    CommissionLedger.original_commission_id == old.id,
                    CommissionLedger.entry_type == "reversal",
                )
            )
            or 0
        )
        already_reversed_rate = rate(
            session.scalar(
                select(func.coalesce(func.sum(-CommissionLedger.rate_percent), 0)).where(
                    CommissionLedger.original_commission_id == old.id,
                    CommissionLedger.entry_type == "reversal",
                )
            )
            or 0
        )
        if cumulative_reversal == original_amount:
            target_amount = money(old.amount_usdt)
            target_rate = rate(old.rate_percent)
        else:
            ratio = cumulative_reversal / original_amount
            target_amount = money(old.amount_usdt * ratio)
            target_rate = rate(old.rate_percent * ratio)
        reversal_amount = -money(target_amount - already_reversed)
        reversal_rate = -rate(target_rate - already_reversed_rate)
        if reversal_amount == 0 and reversal_rate == 0:
            continue
        recipient = session.get(User, old.recipient_user_id) if old.recipient_user_id else None
        row = CommissionLedger(
            deposit_event_id=event.id,
            source_user_id=old.source_user_id,
            recipient_user_id=old.recipient_user_id,
            amount_usdt=reversal_amount,
            recipient_role=old.recipient_role,
            rate_percent=reversal_rate,
            path_depth=old.path_depth,
            entry_type="reversal",
            original_commission_id=old.id,
        )
        session.add(row)
        session.flush()
        if recipient:
            wallet_delta(session, recipient, reversal_amount, "commission_reversal", "commission", row.id)
    return event, False
