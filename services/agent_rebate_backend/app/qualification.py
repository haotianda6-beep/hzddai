from datetime import datetime
from decimal import Decimal

from sqlalchemy import func, select
from sqlalchemy.orm import Session

from app.amounts import money
from app.models import PartnerRole, QualificationReferral, RoleChangeAudit, User

IB_INVITEE_COUNT = 5
IB_INVITEE_DEPOSIT = Decimal("100")


def update_ib_qualification(session: Session, depositor: User, occurred_at: datetime) -> None:
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


def revoke_ib_qualification(session: Session, depositor: User, occurred_at: datetime) -> None:
    if money(depositor.lifetime_deposit_usdt) >= IB_INVITEE_DEPOSIT or not depositor.parent_id:
        return
    parent = session.get(User, depositor.parent_id)
    if not parent:
        return
    qualification = session.scalar(
        select(QualificationReferral).where(
            QualificationReferral.candidate_user_id == parent.id,
            QualificationReferral.invitee_user_id == depositor.id,
        )
    )
    if not qualification:
        return
    session.delete(qualification)
    session.flush()
    count = session.scalar(
        select(func.count(QualificationReferral.id)).where(
            QualificationReferral.candidate_user_id == parent.id
        )
    )
    if (
        parent.role == PartnerRole.IB.value
        and parent.role_source == "auto"
        and int(count or 0) < IB_INVITEE_COUNT
    ):
        parent.role = PartnerRole.RETAIL.value
        parent.role_source = "auto"
        parent.role_assigned_at = occurred_at
        parent.role_assigned_by = "ib_qualification_reversal"
        parent.qualified_at = None
        session.add(
            RoleChangeAudit(
                user_id=parent.id,
                old_role=PartnerRole.IB.value,
                new_role=PartnerRole.RETAIL.value,
                actor="system",
                source="ib_qualification_reversal",
                note="直邀用户累计确认充值冲正后低于100U，IB资格撤销",
            )
        )
