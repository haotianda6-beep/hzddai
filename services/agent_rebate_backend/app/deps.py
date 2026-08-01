import secrets

from fastapi import Depends, HTTPException, Request
from sqlalchemy.orm import Session

from app.config import settings
from app.db import SessionLocal


def get_db():
    db = SessionLocal()
    try:
        yield db
    finally:
        db.close()


def require_platform(request: Request) -> None:
    supplied = (request.headers.get("X-Platform-Secret") or "").strip()
    if not settings.platform_sync_secret:
        raise HTTPException(status_code=503, detail="平台密钥未配置")
    if not supplied or not secrets.compare_digest(supplied, settings.platform_sync_secret):
        raise HTTPException(status_code=401, detail="平台密钥无效")


def require_admin(request: Request) -> None:
    platform = (request.headers.get("X-Platform-Secret") or "").strip()
    admin = (request.headers.get("X-Admin-Token") or request.cookies.get("admin_token") or "").strip()
    if platform and secrets.compare_digest(platform, settings.platform_sync_secret):
        return
    if admin and secrets.compare_digest(admin, settings.admin_token):
        return
    raise HTTPException(status_code=401, detail="需要管理员权限")


Db = Depends(get_db)
