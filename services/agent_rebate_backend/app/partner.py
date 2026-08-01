from __future__ import annotations

from datetime import datetime, timezone
from decimal import Decimal

from sqlalchemy import select
from sqlalchemy.orm import Session

from app.commission import ancestor_chain, is_descendant, money
from app.models import (
    CommissionWalletLedger,
    PartnerRole,
    RoleChangeAudit,
    StudioUpgradeRequest,
    User,
    WithdrawalRequest,
)

VALID_ROLES = {role.value for role in PartnerRole}
WITHDRAW_MIN = Decimal("100")


def now():
    return datetime.now(timezone.utc)


def assign_role(
    session: Session,
    user: User,
    new_role: str,
    actor: str,
    source: str = "admin",
    note: str | None = None,
) -> None:
    new_role = new_role.strip().lower()
    if new_role not in VALID_ROLES:
        raise ValueError("无效身份")
    if new_role == PartnerRole.BRANCH.value:
        user.parent_id = None
    if new_role == PartnerRole.STUDIO.value:
        if any(row.role == PartnerRole.STUDIO.value for row in ancestor_chain(session, user)):
            raise ValueError("工作室不能位于另一工作室伞下")
    old_role = user.role
    if old_role == new_role:
        return
    user.role = new_role
    user.role_source = source
    user.role_assigned_at = now()
    user.role_assigned_by = actor
    if new_role != PartnerRole.IB.value:
        user.qualified_at = None
    session.add(
        RoleChangeAudit(
            user_id=user.id,
            old_role=old_role,
            new_role=new_role,
            actor=actor,
            source=source,
            note=(note or "").strip() or None,
        )
    )


def request_studio(
    session: Session,
    branch: User,
    candidate: User,
    note: str | None = None,
) -> StudioUpgradeRequest:
    if branch.role != PartnerRole.BRANCH.value:
        raise ValueError("只有分公司可以提报工作室")
    if not is_descendant(session, branch.id, candidate):
        raise ValueError("只能提报本分公司伞下用户")
    if candidate.role in {PartnerRole.BRANCH.value, PartnerRole.STUDIO.value}:
        raise ValueError("该用户当前身份不可提报")
    if any(row.role == PartnerRole.STUDIO.value for row in ancestor_chain(session, candidate)):
        raise ValueError("工作室不能再发展工作室")
    pending = session.scalar(
        select(StudioUpgradeRequest).where(
            StudioUpgradeRequest.candidate_user_id == candidate.id,
            StudioUpgradeRequest.status == "pending",
        )
    )
    if pending:
        return pending
    row = StudioUpgradeRequest(
        requested_by_user_id=branch.id,
        candidate_user_id=candidate.id,
        request_note=(note or "").strip() or None,
    )
    session.add(row)
    session.flush()
    return row


def review_studio_request(
    session: Session,
    row: StudioUpgradeRequest,
    approve: bool,
    actor: str,
    note: str | None = None,
) -> None:
    if row.status != "pending":
        raise ValueError("该申请已处理")
    row.status = "approved" if approve else "rejected"
    row.reviewed_by = actor
    row.review_note = (note or "").strip() or None
    row.reviewed_at = now()
    if approve:
        candidate = session.get(User, row.candidate_user_id)
        branch = session.get(User, row.requested_by_user_id)
        if not candidate or not branch or not is_descendant(session, branch.id, candidate):
            raise ValueError("申请人与候选人的伞下关系已失效")
        assign_role(session, candidate, PartnerRole.STUDIO.value, actor, "studio_approval", note)


def request_withdrawal(
    session: Session,
    user: User,
    amount: Decimal,
    address: str,
) -> WithdrawalRequest:
    if user.role == PartnerRole.RETAIL.value:
        raise ValueError("散户没有返佣提现权限")
    amount = money(amount)
    if amount < WITHDRAW_MIN:
        raise ValueError("最低提现金额为100U")
    if amount > money(user.rebate_balance_usdt):
        raise ValueError("可提现余额不足")
    address = address.strip()
    if len(address) < 20 or len(address) > 128:
        raise ValueError("TRC20地址格式无效")
    user.rebate_balance_usdt = money(user.rebate_balance_usdt - amount)
    user.frozen_balance_usdt = money(user.frozen_balance_usdt + amount)
    row = WithdrawalRequest(
        user_id=user.id,
        amount_usdt=amount,
        network="TRC20",
        address=address,
        status="pending",
    )
    session.add(row)
    session.flush()
    session.add(
        CommissionWalletLedger(
            user_id=user.id,
            delta_usdt=-amount,
            available_after_usdt=user.rebate_balance_usdt,
            frozen_after_usdt=user.frozen_balance_usdt,
            reason="withdrawal_frozen",
            reference_type="withdrawal",
            reference_id=row.id,
        )
    )
    return row


def review_withdrawal(
    session: Session,
    row: WithdrawalRequest,
    action: str,
    actor: str,
    note: str | None = None,
) -> None:
    action = action.strip().lower()
    allowed = {
        "pending": {"approve", "reject"},
        "approved": {"paid", "reject"},
    }
    if action not in allowed.get(row.status, set()):
        raise ValueError("当前提现状态不允许此操作")
    user = session.get(User, row.user_id)
    if not user:
        raise ValueError("提现用户不存在")
    row.reviewed_by = actor
    row.review_note = (note or "").strip() or None
    row.reviewed_at = now()
    if action == "approve":
        row.status = "approved"
        return
    if action == "paid":
        row.status = "paid"
        row.paid_at = now()
        user.frozen_balance_usdt = money(user.frozen_balance_usdt - row.amount_usdt)
        reason = "withdrawal_paid"
    else:
        row.status = "rejected"
        user.frozen_balance_usdt = money(user.frozen_balance_usdt - row.amount_usdt)
        user.rebate_balance_usdt = money(user.rebate_balance_usdt + row.amount_usdt)
        reason = "withdrawal_rejected"
    session.add(
        CommissionWalletLedger(
            user_id=user.id,
            delta_usdt=Decimal("0") if action == "paid" else row.amount_usdt,
            available_after_usdt=user.rebate_balance_usdt,
            frozen_after_usdt=user.frozen_balance_usdt,
            reason=reason,
            reference_type="withdrawal",
            reference_id=row.id,
        )
    )
