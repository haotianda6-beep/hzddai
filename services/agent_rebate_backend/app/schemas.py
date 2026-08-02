from __future__ import annotations

from datetime import datetime
from typing import Optional

from pydantic import BaseModel, Field, model_validator


class UserSyncItem(BaseModel):
    external_uid: str = Field(min_length=1, max_length=64)
    nickname: str = ""
    parent_external_uid: Optional[str] = None


class UserSyncBody(BaseModel):
    users: Optional[list[UserSyncItem]] = None
    replace_missing: bool = False
    external_uid: Optional[str] = None
    nickname: str = ""
    parent_external_uid: Optional[str] = None

    @model_validator(mode="after")
    def validate_mode(self):
        if self.users:
            return self
        if not (self.external_uid or "").strip():
            raise ValueError("请提供用户")
        return self

    def items(self) -> list[UserSyncItem]:
        if self.users:
            return self.users
        return [
            UserSyncItem(
                external_uid=self.external_uid or "",
                nickname=self.nickname,
                parent_external_uid=self.parent_external_uid,
            )
        ]


class DepositBody(BaseModel):
    platform_user_id: str = Field(min_length=1, max_length=64)
    amount_usdt: str = Field(min_length=1, max_length=64)
    external_ref: str = Field(min_length=1, max_length=160)
    source: str = Field(default="confirmed_deposit", max_length=40)
    note: Optional[str] = Field(default=None, max_length=500)
    occurred_at: Optional[datetime] = None


class ReversalBody(BaseModel):
    original_external_ref: str = Field(min_length=1, max_length=160)
    amount_usdt: str = Field(min_length=1, max_length=64)
    external_ref: str = Field(min_length=1, max_length=160)
    note: Optional[str] = Field(default=None, max_length=500)


class WithdrawalBody(BaseModel):
    platform_user_id: str = Field(min_length=1, max_length=64)
    amount_usdt: str = Field(min_length=1, max_length=64)
    network: str = "TRC20"
    address: str = Field(min_length=20, max_length=128)


class StudioRequestBody(BaseModel):
    branch_platform_user_id: str = Field(min_length=1, max_length=64)
    candidate_platform_user_id: str = Field(min_length=1, max_length=64)
    note: Optional[str] = Field(default=None, max_length=500)


class RoleBody(BaseModel):
    platform_user_id: str = Field(min_length=1, max_length=64)
    role: str
    actor: str = Field(default="platform_admin", max_length=64)
    note: Optional[str] = Field(default=None, max_length=500)


class ReviewBody(BaseModel):
    action: str
    actor: str = Field(default="platform_admin", max_length=64)
    note: Optional[str] = Field(default=None, max_length=500)
