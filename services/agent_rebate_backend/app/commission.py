from __future__ import annotations

from datetime import datetime, timezone
from decimal import Decimal

from sqlalchemy import func, select
from sqlalchemy.orm import Session

from app.models import (
    ROLE_RATE,
    CommissionLedger,
    CommissionWalletLedger,
    DepositEvent,
    PartnerRole,
    QualificationReferral,
    RoleChangeAudit,
    User,
)

MONEY = Decimal("0.00000001")
IB_INVITEE_COUNT = 5
IB_INVITEE_DEPOSIT = Decimal("100")


def money(value: Decimal | str | int | float) -> Decimal:
    return Decimal(str(value)).quantize(MONEY)


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
    rows: list[CommissionLedger] = []
    for depth, ancestor in enumerate(ancestor_chain(session, depositor), start=1):
        cap = ROLE_RATE.get(ancestor.role, Decimal("0"))
        if cap <= absorbed:
            continue
        delta_rate = cap - absorbed
        amount = money(event.amount_usdt * delta_rate / Decimal("100"))
        absorbed = cap
        if amount <= 0:
            continue
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


def _update_ib_qualification(session: Session, depositor: User, occurred_at: datetime) -> None:
    parent = session.get(User, depositor.parent_id) if depositor.parent_id else None
    if not parent or parent.role != PartnerRole.RETAIL.value:
        return
    if money(depositor.lifetime_deposit_usdt) < IB_INVITEE_DEPOSIT:
        return
    existing = session.scalar(
        select(QualificationReferral.id).where(
            QualificationReferral.candidate_user_id == parent.id,
            QualificationReferral.invitee_user_id == depositor.id,
        )
    )
    if existing is None:
        session.add(
            QualificationReferral(
                candidate_user_id=parent.id,
                invitee_user_id=depositor.id,
                threshold_usdt=IB_INVITEE_DEPOSIT,
                reached_at=occurred_at,
            )
        )
        session.flush()
    count = session.scalar(
        select(func.count(QualificationReferral.id)).where(
            QualificationReferral.candidate_user_id == parent.id
        )
    )
    if int(count or 0) < IB_INVITEE_COUNT:
        return
    old_role = parent.role
    parent.role = PartnerRole.IB.value
    parent.role_source = "auto"
    parent.role_assigned_at = occurred_at
    parent.role_assigned_by = "ib_qualification"
    parent.qualified_at = occurred_at
    session.add(
        RoleChangeAudit(
            user_id=parent.id,
            old_role=old_role,
            new_role=parent.role,
            actor="system",
            source="ib_qualification",
            note="5名直邀用户分别累计确认充值达到100U",
        )
    )


def record_deposit(
    session: Session,
    user: User,
    amount: Decimal,
    external_ref: str,
    source: str,
    note: str | None = None,
    occurred_at: datetime | None = None,
) -> tuple[DepositEvent, bool]:
    existing = session.scalar(select(DepositEvent).where(DepositEvent.external_ref == external_ref))
    if existing:
        return existing, True
    amount = money(amount)
    if amount <= 0:
        raise ValueError("确认充值金额必须大于0")
    if not external_ref.strip():
        raise ValueError("external_ref不能为空")
    when = occurred_at or utcnow()
    event = DepositEvent(
        external_ref=external_ref.strip(),
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
    _update_ib_qualification(session, user, when)
    return event, False


def reverse_deposit(
    session: Session,
    original: DepositEvent,
    amount: Decimal,
    external_ref: str,
    note: str | None = None,
) -> tuple[DepositEvent, bool]:
    existing = session.scalar(select(DepositEvent).where(DepositEvent.external_ref == external_ref))
    if existing:
        return existing, True
    amount = money(amount)
    if original.event_type != "deposit" or amount <= 0:
        raise ValueError("冲正参数无效")
    reversed_total = session.scalar(
        select(func.coalesce(func.sum(-DepositEvent.amount_usdt), 0)).where(
            DepositEvent.original_event_id == original.id,
            DepositEvent.event_type == "reversal",
        )
    )
    if amount > money(original.amount_usdt - Decimal(str(reversed_total or 0))):
        raise ValueError("冲正金额超过原确认充值可冲正余额")
    event = DepositEvent(
        external_ref=external_ref.strip(),
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
    depositor = session.get(User, original.user_id)
    depositor.lifetime_deposit_usdt = money(max(Decimal("0"), depositor.lifetime_deposit_usdt - amount))
    original_rows = session.scalars(
        select(CommissionLedger).where(
            CommissionLedger.deposit_event_id == original.id,
            CommissionLedger.entry_type == "commission",
        )
    ).all()
    ratio = amount / money(original.amount_usdt)
    for old in original_rows:
        recipient = session.get(User, old.recipient_user_id)
        reversal_amount = -money(old.amount_usdt * ratio)
        row = CommissionLedger(
            deposit_event_id=event.id,
            source_user_id=old.source_user_id,
            recipient_user_id=old.recipient_user_id,
            amount_usdt=reversal_amount,
            recipient_role=old.recipient_role,
            rate_percent=-money(old.rate_percent * ratio),
            path_depth=old.path_depth,
            entry_type="reversal",
            original_commission_id=old.id,
        )
        session.add(row)
        session.flush()
        wallet_delta(session, recipient, reversal_amount, "commission_reversal", "commission", row.id)
    return event, False
