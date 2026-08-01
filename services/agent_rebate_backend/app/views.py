from __future__ import annotations

from decimal import Decimal

from sqlalchemy import func, select
from sqlalchemy.orm import Session

from app.models import (
    ROLE_RATE,
    CommissionLedger,
    DepositEvent,
    QualificationReferral,
    RoleChangeAudit,
    StudioUpgradeRequest,
    User,
    WithdrawalRequest,
)


def fmt(value) -> str:
    raw = format(Decimal(str(value or 0)), "f")
    return raw.rstrip("0").rstrip(".") if "." in raw else raw


def iso(value):
    return value.isoformat() if value else None


def user_value(users: dict[int, User], user_id: int | None, field: str) -> str:
    user = users.get(user_id) if user_id else None
    if not user:
        return "已删除用户"
    return str(getattr(user, field) or "")


def descendants(session: Session, root_id: int) -> list[tuple[User, int]]:
    all_users = session.scalars(select(User)).all()
    children: dict[int, list[User]] = {}
    for user in all_users:
        if user.parent_id:
            children.setdefault(user.parent_id, []).append(user)
    result: list[tuple[User, int]] = []
    queue = [(child, 1) for child in children.get(root_id, [])]
    seen = {root_id}
    while queue:
        user, depth = queue.pop(0)
        if user.id in seen:
            continue
        seen.add(user.id)
        result.append((user, depth))
        queue.extend((child, depth + 1) for child in children.get(user.id, []))
    return result


def user_dashboard(session: Session, user: User, limit: int = 200) -> dict:
    network = descendants(session, user.id)
    network_ids = [row.id for row, _ in network]
    qualified_ids = set(
        session.scalars(
            select(QualificationReferral.invitee_user_id).where(
                QualificationReferral.candidate_user_id == user.id
            )
        ).all()
    )
    direct = session.scalars(select(User).where(User.parent_id == user.id).order_by(User.created_at)).all()
    commission_rows = session.scalars(
        select(CommissionLedger)
        .where(CommissionLedger.recipient_user_id == user.id)
        .order_by(CommissionLedger.id.desc())
        .limit(limit)
    ).all()
    event_ids = [row.deposit_event_id for row in commission_rows]
    events = {
        row.id: row
        for row in session.scalars(select(DepositEvent).where(DepositEvent.id.in_(event_ids))).all()
    } if event_ids else {}
    source_ids = [row.source_user_id for row in commission_rows]
    sources = {
        row.id: row
        for row in session.scalars(select(User).where(User.id.in_(source_ids))).all()
    } if source_ids else {}
    deposits = []
    if network_ids:
        rows = session.scalars(
            select(DepositEvent)
            .where(DepositEvent.user_id.in_(network_ids))
            .order_by(DepositEvent.id.desc())
            .limit(limit)
        ).all()
        users = {row.id: row for row, _ in network}
        deposits = [
            {
                "id": row.id,
                "user_uid": users[row.user_id].external_uid,
                "nickname": users[row.user_id].nickname,
                "amount_usdt": fmt(row.amount_usdt),
                "event_type": row.event_type,
                "source": row.source,
                "occurred_at": iso(row.occurred_at),
            }
            for row in rows
        ]
    withdrawals = session.scalars(
        select(WithdrawalRequest)
        .where(WithdrawalRequest.user_id == user.id)
        .order_by(WithdrawalRequest.id.desc())
        .limit(100)
    ).all()
    return {
        "ok": True,
        "user": {
            "platform_user_id": user.external_uid,
            "nickname": user.nickname,
            "role": user.role,
            "role_rate_percent": fmt(ROLE_RATE[user.role]),
            "available_balance_usdt": fmt(user.rebate_balance_usdt),
            "frozen_balance_usdt": fmt(user.frozen_balance_usdt),
            "lifetime_deposit_usdt": fmt(user.lifetime_deposit_usdt),
            "qualified_at": iso(user.qualified_at),
        },
        "ib_progress": {
            "qualified_count": len(qualified_ids),
            "required_count": 5,
            "required_each_usdt": "100",
            "excluded_total_usdt": str(len(qualified_ids) * 100),
            "direct_users": [
                {
                    "platform_user_id": row.external_uid,
                    "nickname": row.nickname,
                    "deposit_usdt": fmt(row.lifetime_deposit_usdt),
                    "qualified": row.id in qualified_ids,
                }
                for row in direct
            ],
        },
        "network": [
            {
                "platform_user_id": row.external_uid,
                "parent_platform_user_id": session.get(User, row.parent_id).external_uid if row.parent_id else None,
                "nickname": row.nickname,
                "role": row.role,
                "depth": depth,
                "deposit_usdt": fmt(row.lifetime_deposit_usdt),
            }
            for row, depth in network
        ],
        "deposits": deposits,
        "commissions": [
            {
                "id": row.id,
                "source_platform_user_id": user_value(sources, row.source_user_id, "external_uid"),
                "source_nickname": user_value(sources, row.source_user_id, "nickname"),
                "deposit_usdt": fmt(events[row.deposit_event_id].amount_usdt),
                "amount_usdt": fmt(row.amount_usdt),
                "rate_percent": fmt(row.rate_percent),
                "role": row.recipient_role,
                "entry_type": row.entry_type,
                "created_at": iso(row.created_at),
            }
            for row in commission_rows
        ],
        "withdrawals": [
            {
                "id": row.id,
                "amount_usdt": fmt(row.amount_usdt),
                "network": row.network,
                "address": row.address,
                "status": row.status,
                "requested_at": iso(row.requested_at),
                "review_note": row.review_note,
            }
            for row in withdrawals
        ],
    }


def admin_dashboard(session: Session, limit: int = 300) -> dict:
    users = session.scalars(select(User).order_by(User.id.desc())).all()
    by_id = {row.id: row for row in users}
    deposits = session.scalars(select(DepositEvent).order_by(DepositEvent.id.desc()).limit(limit)).all()
    commissions = session.scalars(
        select(CommissionLedger).order_by(CommissionLedger.id.desc()).limit(limit)
    ).all()
    withdrawals = session.scalars(
        select(WithdrawalRequest).order_by(WithdrawalRequest.id.desc()).limit(limit)
    ).all()
    studio_requests = session.scalars(
        select(StudioUpgradeRequest).order_by(StudioUpgradeRequest.id.desc()).limit(limit)
    ).all()
    role_audits = session.scalars(
        select(RoleChangeAudit).order_by(RoleChangeAudit.id.desc()).limit(limit)
    ).all()
    total_deposits = session.scalar(select(func.coalesce(func.sum(DepositEvent.amount_usdt), 0)))
    total_commission = session.scalar(select(func.coalesce(func.sum(CommissionLedger.amount_usdt), 0)))
    total_available = session.scalar(select(func.coalesce(func.sum(User.rebate_balance_usdt), 0)))
    total_frozen = session.scalar(select(func.coalesce(func.sum(User.frozen_balance_usdt), 0)))
    role_counts = {
        role: sum(1 for row in users if row.role == role)
        for role in ROLE_RATE
    }
    return {
        "ok": True,
        "summary": {
            "users": len(users),
            "roles": role_counts,
            "confirmed_deposits_usdt": fmt(total_deposits),
            "allocated_commission_usdt": fmt(total_commission),
            "platform_remainder_usdt": fmt(Decimal(str(total_deposits or 0)) - Decimal(str(total_commission or 0))),
            "available_liability_usdt": fmt(total_available),
            "frozen_liability_usdt": fmt(total_frozen),
        },
        "users": [
            {
                "platform_user_id": row.external_uid,
                "nickname": row.nickname,
                "parent_platform_user_id": by_id[row.parent_id].external_uid if row.parent_id else None,
                "role": row.role,
                "deposit_usdt": fmt(row.lifetime_deposit_usdt),
                "available_usdt": fmt(row.rebate_balance_usdt),
                "frozen_usdt": fmt(row.frozen_balance_usdt),
            }
            for row in users
        ],
        "deposits": [
            {
                "id": row.id,
                "platform_user_id": user_value(by_id, row.user_id, "external_uid"),
                "nickname": user_value(by_id, row.user_id, "nickname"),
                "amount_usdt": fmt(row.amount_usdt),
                "event_type": row.event_type,
                "source": row.source,
                "external_ref": row.external_ref,
                "occurred_at": iso(row.occurred_at),
            }
            for row in deposits
        ],
        "commissions": [
            {
                "id": row.id,
                "source_platform_user_id": user_value(by_id, row.source_user_id, "external_uid"),
                "recipient_platform_user_id": user_value(by_id, row.recipient_user_id, "external_uid"),
                "amount_usdt": fmt(row.amount_usdt),
                "rate_percent": fmt(row.rate_percent),
                "recipient_role": row.recipient_role,
                "entry_type": row.entry_type,
                "created_at": iso(row.created_at),
            }
            for row in commissions
        ],
        "withdrawals": [
            {
                "id": row.id,
                "platform_user_id": by_id[row.user_id].external_uid,
                "nickname": by_id[row.user_id].nickname,
                "amount_usdt": fmt(row.amount_usdt),
                "network": row.network,
                "address": row.address,
                "status": row.status,
                "requested_at": iso(row.requested_at),
            }
            for row in withdrawals
        ],
        "studio_requests": [
            {
                "id": row.id,
                "branch_platform_user_id": by_id[row.requested_by_user_id].external_uid,
                "candidate_platform_user_id": by_id[row.candidate_user_id].external_uid,
                "candidate_nickname": by_id[row.candidate_user_id].nickname,
                "status": row.status,
                "request_note": row.request_note,
                "review_note": row.review_note,
                "created_at": iso(row.created_at),
            }
            for row in studio_requests
        ],
        "role_audits": [
            {
                "id": row.id,
                "platform_user_id": by_id[row.user_id].external_uid,
                "old_role": row.old_role,
                "new_role": row.new_role,
                "actor": row.actor,
                "source": row.source,
                "note": row.note,
                "created_at": iso(row.created_at),
            }
            for row in role_audits
        ],
    }
