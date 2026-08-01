from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy import select
from sqlalchemy.orm import Session

from app.deps import get_db, require_admin
from app.models import StudioUpgradeRequest, User, WithdrawalRequest
from app.partner import assign_role, review_studio_request, review_withdrawal
from app.schemas import ReviewBody, RoleBody
from app.views import admin_dashboard

router = APIRouter(prefix="/api/admin", dependencies=[Depends(require_admin)])


def user_by_uid(db: Session, uid: str) -> User:
    user = db.scalar(select(User).where(User.external_uid == uid.strip()))
    if not user:
        raise HTTPException(status_code=404, detail="用户不存在")
    return user


@router.get("/dashboard")
def dashboard(limit: int = Query(300, ge=1, le=1000), db: Session = Depends(get_db)):
    return admin_dashboard(db, limit)


@router.post("/roles")
def set_role(body: RoleBody, db: Session = Depends(get_db)):
    try:
        user = user_by_uid(db, body.platform_user_id)
        assign_role(db, user, body.role, body.actor, "admin", body.note)
        db.commit()
    except ValueError as exc:
        db.rollback()
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    return {"ok": True, "platform_user_id": user.external_uid, "role": user.role}


@router.post("/studio-requests/{request_id}/review")
def studio_review(request_id: int, body: ReviewBody, db: Session = Depends(get_db)):
    row = db.get(StudioUpgradeRequest, request_id)
    if not row:
        raise HTTPException(status_code=404, detail="工作室申请不存在")
    if body.action not in {"approve", "reject"}:
        raise HTTPException(status_code=400, detail="action必须是approve或reject")
    try:
        review_studio_request(db, row, body.action == "approve", body.actor, body.note)
        db.commit()
    except ValueError as exc:
        db.rollback()
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    return {"ok": True, "request_id": row.id, "status": row.status}


@router.post("/withdrawals/{withdrawal_id}/review")
def withdrawal_review(withdrawal_id: int, body: ReviewBody, db: Session = Depends(get_db)):
    row = db.get(WithdrawalRequest, withdrawal_id)
    if not row:
        raise HTTPException(status_code=404, detail="提现申请不存在")
    try:
        review_withdrawal(db, row, body.action, body.actor, body.note)
        db.commit()
    except ValueError as exc:
        db.rollback()
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    return {"ok": True, "withdrawal_id": row.id, "status": row.status}
