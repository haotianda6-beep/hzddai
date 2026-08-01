from __future__ import annotations

from decimal import Decimal, InvalidOperation

from fastapi import APIRouter, Depends, HTTPException, Query, Request
from sqlalchemy import select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from app.commission import record_deposit, reverse_deposit
from app.deps import get_db, require_platform
from app.models import DepositEvent, User
from app.partner import request_studio, request_withdrawal
from app.platform_users import order_sync_batch, prune_missing_users, upsert_user
from app.schemas import DepositBody, ReversalBody, StudioRequestBody, UserSyncBody, WithdrawalBody
from app.views import user_dashboard

router = APIRouter(prefix="/api/platform", dependencies=[Depends(require_platform)])


def user_by_uid(db: Session, uid: str) -> User:
    user = db.scalar(select(User).where(User.external_uid == uid.strip()))
    if not user:
        raise HTTPException(status_code=404, detail="用户尚未同步到返佣系统")
    return user


def decimal_amount(raw: str) -> Decimal:
    try:
        return Decimal(raw)
    except (InvalidOperation, ValueError) as exc:
        raise HTTPException(status_code=400, detail="金额格式无效") from exc


@router.post("/user-sync")
def sync_users(body: UserSyncBody, db: Session = Depends(get_db)):
    items = body.items()
    if len(items) > 2000:
        raise HTTPException(status_code=400, detail="单次最多同步2000名用户")
    try:
        ordered = order_sync_batch(items, lambda x: x.external_uid, lambda x: x.parent_external_uid)
        rows = [
            upsert_user(db, item.external_uid, item.nickname, item.parent_external_uid)
            for item in ordered
        ]
        pruned = 0
        if body.replace_missing:
            pruned = prune_missing_users(db, {item.external_uid.strip() for item in items})
        db.commit()
    except ValueError as exc:
        db.rollback()
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    return {"ok": True, "count": len(rows), "pruned": pruned}


@router.post("/deposits")
def confirmed_deposit(body: DepositBody, db: Session = Depends(get_db)):
    user = user_by_uid(db, body.platform_user_id)
    try:
        event, duplicate = record_deposit(
            db,
            user,
            decimal_amount(body.amount_usdt),
            body.external_ref,
            body.source,
            body.note,
            body.occurred_at,
        )
        db.commit()
    except IntegrityError:
        db.rollback()
        event = db.scalar(select(DepositEvent).where(DepositEvent.external_ref == body.external_ref))
        if not event:
            raise
        duplicate = True
    except ValueError as exc:
        db.rollback()
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    return {"ok": True, "duplicate": duplicate, "deposit_event_id": event.id}


@router.post("/deposit-reversals")
def deposit_reversal(body: ReversalBody, db: Session = Depends(get_db)):
    original = db.scalar(
        select(DepositEvent).where(DepositEvent.external_ref == body.original_external_ref)
    )
    if not original:
        raise HTTPException(status_code=404, detail="原确认充值不存在")
    try:
        event, duplicate = reverse_deposit(
            db, original, decimal_amount(body.amount_usdt), body.external_ref, body.note
        )
        db.commit()
    except IntegrityError:
        db.rollback()
        event = db.scalar(select(DepositEvent).where(DepositEvent.external_ref == body.external_ref))
        if not event:
            raise
        duplicate = True
    except ValueError as exc:
        db.rollback()
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    return {"ok": True, "duplicate": duplicate, "reversal_event_id": event.id}


@router.get("/dashboard")
def dashboard(
    platform_user_id: str = Query(...),
    limit: int = Query(200, ge=1, le=500),
    db: Session = Depends(get_db),
):
    return user_dashboard(db, user_by_uid(db, platform_user_id), limit)


@router.post("/withdrawals")
def create_withdrawal(body: WithdrawalBody, db: Session = Depends(get_db)):
    if body.network.strip().upper() != "TRC20":
        raise HTTPException(status_code=400, detail="当前仅支持TRC20")
    try:
        row = request_withdrawal(
            db,
            user_by_uid(db, body.platform_user_id),
            decimal_amount(body.amount_usdt),
            body.address,
        )
        db.commit()
    except ValueError as exc:
        db.rollback()
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    return {"ok": True, "withdrawal_id": row.id, "status": row.status}


@router.post("/studio-requests")
def create_studio_request(body: StudioRequestBody, db: Session = Depends(get_db)):
    try:
        row = request_studio(
            db,
            user_by_uid(db, body.branch_platform_user_id),
            user_by_uid(db, body.candidate_platform_user_id),
            body.note,
        )
        db.commit()
    except ValueError as exc:
        db.rollback()
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    return {"ok": True, "request_id": row.id, "status": row.status}
