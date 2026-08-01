from __future__ import annotations

from collections import defaultdict, deque
from decimal import Decimal
from typing import Optional, TypeVar

import secrets
from sqlalchemy import delete, or_, select, update
from sqlalchemy.orm import Session

from app.commission import ancestor_chain
from app.models import (
    CommissionLedger,
    CommissionWalletLedger,
    DepositEvent,
    PartnerRole,
    QualificationReferral,
    RoleChangeAudit,
    StudioUpgradeRequest,
    User,
    WithdrawalRequest,
)


def resolve_parent_id(session: Session, external_uid: Optional[str]) -> Optional[int]:
    external_uid = (external_uid or "").strip()
    if not external_uid:
        return None
    parent = session.scalar(select(User).where(User.external_uid == external_uid))
    if not parent:
        raise ValueError(f"邀请人不存在: {external_uid}")
    return parent.id


def upsert_user(
    session: Session,
    external_uid: str,
    nickname: str,
    parent_external_uid: str | None,
) -> User:
    external_uid = external_uid.strip()
    if not external_uid:
        raise ValueError("external_uid不能为空")
    parent_id = resolve_parent_id(session, parent_external_uid)
    user = session.scalar(select(User).where(User.external_uid == external_uid))
    if not user:
        user = User(
            external_uid=external_uid,
            nickname=nickname.strip(),
            parent_id=parent_id,
            api_token=secrets.token_urlsafe(32),
            role=PartnerRole.RETAIL.value,
            rebate_balance_usdt=Decimal("0"),
            frozen_balance_usdt=Decimal("0"),
            lifetime_deposit_usdt=Decimal("0"),
        )
        session.add(user)
        session.flush()
        return user
    user.nickname = nickname.strip() or user.nickname
    if user.role != PartnerRole.BRANCH.value:
        old_parent = user.parent_id
        user.parent_id = parent_id
        try:
            if any(row.id == user.id for row in ancestor_chain(session, user)):
                raise ValueError("邀请关系存在循环")
        except Exception:
            user.parent_id = old_parent
            raise
    return user


T = TypeVar("T")


def order_sync_batch(items: list[T], get_uid, get_parent_uid) -> list[T]:
    uids = [get_uid(item).strip() for item in items]
    if len(set(uids)) != len(uids):
        raise ValueError("批次中存在重复用户")
    uid_set = set(uids)
    item_by_uid = dict(zip(uids, items, strict=True))
    children: dict[str, list[str]] = defaultdict(list)
    indegree = {uid: 0 for uid in uids}
    for item in items:
        uid = get_uid(item).strip()
        parent_uid = (get_parent_uid(item) or "").strip()
        if parent_uid and parent_uid in uid_set:
            children[parent_uid].append(uid)
            indegree[uid] += 1
    queue = deque(uid for uid in uids if indegree[uid] == 0)
    ordered: list[str] = []
    while queue:
        uid = queue.popleft()
        ordered.append(uid)
        for child in children.get(uid, []):
            indegree[child] -= 1
            if indegree[child] == 0:
                queue.append(child)
    if len(ordered) != len(uids):
        raise ValueError("邀请关系存在循环")
    return [item_by_uid[uid] for uid in ordered]


def prune_missing_users(session: Session, active_external_uids: set[str]) -> int:
    rows = session.scalars(select(User).where(User.external_uid.notin_(active_external_uids))).all()
    ids = [row.id for row in rows]
    if not ids:
        return 0
    session.execute(update(User).where(User.parent_id.in_(ids)).values(parent_id=None))
    session.execute(update(DepositEvent).where(DepositEvent.user_id.in_(ids)).values(user_id=None))
    session.execute(
        update(CommissionLedger).where(CommissionLedger.source_user_id.in_(ids)).values(source_user_id=None)
    )
    session.execute(
        update(CommissionLedger)
        .where(CommissionLedger.recipient_user_id.in_(ids))
        .values(recipient_user_id=None)
    )
    session.execute(delete(CommissionWalletLedger).where(CommissionWalletLedger.user_id.in_(ids)))
    session.execute(
        delete(QualificationReferral).where(
            or_(
                QualificationReferral.candidate_user_id.in_(ids),
                QualificationReferral.invitee_user_id.in_(ids),
            )
        )
    )
    session.execute(
        delete(StudioUpgradeRequest).where(
            or_(
                StudioUpgradeRequest.requested_by_user_id.in_(ids),
                StudioUpgradeRequest.candidate_user_id.in_(ids),
            )
        )
    )
    session.execute(delete(RoleChangeAudit).where(RoleChangeAudit.user_id.in_(ids)))
    session.execute(delete(WithdrawalRequest).where(WithdrawalRequest.user_id.in_(ids)))
    session.execute(delete(User).where(User.id.in_(ids)))
    session.flush()
    return len(ids)
