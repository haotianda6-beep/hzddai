from __future__ import annotations

import enum
from datetime import datetime
from decimal import Decimal
from typing import Optional

from sqlalchemy import DateTime, ForeignKey, Index, Numeric, String, Text, UniqueConstraint, func
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column, relationship


class Base(DeclarativeBase):
    pass


class PartnerRole(str, enum.Enum):
    RETAIL = "retail"
    IB = "ib"
    STUDIO = "studio"
    BRANCH = "branch"


ROLE_RATE = {
    PartnerRole.RETAIL.value: Decimal("0"),
    PartnerRole.IB.value: Decimal("20"),
    PartnerRole.STUDIO.value: Decimal("50"),
    PartnerRole.BRANCH.value: Decimal("70"),
}


class User(Base):
    __tablename__ = "users"

    id: Mapped[int] = mapped_column(primary_key=True, autoincrement=True)
    external_uid: Mapped[str] = mapped_column(String(64), unique=True, index=True)
    api_token: Mapped[Optional[str]] = mapped_column(String(128), unique=True, index=True)
    nickname: Mapped[str] = mapped_column(String(128), default="")
    parent_id: Mapped[Optional[int]] = mapped_column(ForeignKey("users.id"), index=True)
    role: Mapped[str] = mapped_column(String(16), default=PartnerRole.RETAIL.value, index=True)
    role_source: Mapped[str] = mapped_column(String(16), default="system")
    role_assigned_at: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True))
    role_assigned_by: Mapped[Optional[str]] = mapped_column(String(64))
    qualified_at: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True))
    rebate_balance_usdt: Mapped[Decimal] = mapped_column(Numeric(24, 8), default=Decimal("0"))
    frozen_balance_usdt: Mapped[Decimal] = mapped_column(Numeric(24, 8), default=Decimal("0"))
    lifetime_deposit_usdt: Mapped[Decimal] = mapped_column(Numeric(24, 8), default=Decimal("0"))
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=func.now(), onupdate=func.now()
    )

    parent: Mapped[Optional["User"]] = relationship(
        "User", remote_side=[id], foreign_keys=[parent_id], back_populates="children"
    )
    children: Mapped[list["User"]] = relationship("User", back_populates="parent")


class DepositEvent(Base):
    __tablename__ = "deposit_events"

    id: Mapped[int] = mapped_column(primary_key=True, autoincrement=True)
    external_ref: Mapped[str] = mapped_column(String(160), unique=True, index=True)
    user_id: Mapped[Optional[int]] = mapped_column(ForeignKey("users.id"), nullable=True, index=True)
    amount_usdt: Mapped[Decimal] = mapped_column(Numeric(24, 8))
    event_type: Mapped[str] = mapped_column(String(16), default="deposit", index=True)
    original_event_id: Mapped[Optional[int]] = mapped_column(ForeignKey("deposit_events.id"), index=True)
    source: Mapped[str] = mapped_column(String(40))
    note: Mapped[Optional[str]] = mapped_column(Text)
    occurred_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


class CommissionLedger(Base):
    __tablename__ = "commission_ledger"
    __table_args__ = (
        UniqueConstraint("deposit_event_id", "recipient_user_id", name="uq_commission_event_recipient"),
        Index("ix_commission_recipient_created", "recipient_user_id", "created_at"),
    )

    id: Mapped[int] = mapped_column(primary_key=True, autoincrement=True)
    deposit_event_id: Mapped[int] = mapped_column(ForeignKey("deposit_events.id"), index=True)
    source_user_id: Mapped[Optional[int]] = mapped_column(ForeignKey("users.id"), nullable=True, index=True)
    recipient_user_id: Mapped[Optional[int]] = mapped_column(ForeignKey("users.id"), nullable=True, index=True)
    amount_usdt: Mapped[Decimal] = mapped_column(Numeric(24, 8))
    recipient_role: Mapped[str] = mapped_column(String(16))
    rate_percent: Mapped[Decimal] = mapped_column(Numeric(8, 4))
    path_depth: Mapped[int] = mapped_column()
    entry_type: Mapped[str] = mapped_column(String(16), default="commission")
    original_commission_id: Mapped[Optional[int]] = mapped_column(
        ForeignKey("commission_ledger.id"), index=True
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


class CommissionWalletLedger(Base):
    __tablename__ = "commission_wallet_ledger"

    id: Mapped[int] = mapped_column(primary_key=True, autoincrement=True)
    user_id: Mapped[int] = mapped_column(ForeignKey("users.id"), index=True)
    delta_usdt: Mapped[Decimal] = mapped_column(Numeric(24, 8))
    available_after_usdt: Mapped[Decimal] = mapped_column(Numeric(24, 8))
    frozen_after_usdt: Mapped[Decimal] = mapped_column(Numeric(24, 8))
    reason: Mapped[str] = mapped_column(String(32), index=True)
    reference_type: Mapped[str] = mapped_column(String(32))
    reference_id: Mapped[int] = mapped_column(index=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


class QualificationReferral(Base):
    __tablename__ = "qualification_referrals"
    __table_args__ = (UniqueConstraint("candidate_user_id", "invitee_user_id"),)

    id: Mapped[int] = mapped_column(primary_key=True, autoincrement=True)
    candidate_user_id: Mapped[int] = mapped_column(ForeignKey("users.id"), index=True)
    invitee_user_id: Mapped[int] = mapped_column(ForeignKey("users.id"), index=True)
    threshold_usdt: Mapped[Decimal] = mapped_column(Numeric(24, 8), default=Decimal("100"))
    reached_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


class StudioUpgradeRequest(Base):
    __tablename__ = "studio_upgrade_requests"
    __table_args__ = (Index("ix_studio_request_status_created", "status", "created_at"),)

    id: Mapped[int] = mapped_column(primary_key=True, autoincrement=True)
    requested_by_user_id: Mapped[int] = mapped_column(ForeignKey("users.id"), index=True)
    candidate_user_id: Mapped[int] = mapped_column(ForeignKey("users.id"), index=True)
    status: Mapped[str] = mapped_column(String(16), default="pending", index=True)
    request_note: Mapped[Optional[str]] = mapped_column(Text)
    reviewed_by: Mapped[Optional[str]] = mapped_column(String(64))
    review_note: Mapped[Optional[str]] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
    reviewed_at: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True))


class RoleChangeAudit(Base):
    __tablename__ = "role_change_audits"

    id: Mapped[int] = mapped_column(primary_key=True, autoincrement=True)
    user_id: Mapped[int] = mapped_column(ForeignKey("users.id"), index=True)
    old_role: Mapped[str] = mapped_column(String(16))
    new_role: Mapped[str] = mapped_column(String(16))
    actor: Mapped[str] = mapped_column(String(64))
    source: Mapped[str] = mapped_column(String(24))
    note: Mapped[Optional[str]] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


class WithdrawalRequest(Base):
    __tablename__ = "withdrawal_requests"

    id: Mapped[int] = mapped_column(primary_key=True, autoincrement=True)
    user_id: Mapped[int] = mapped_column(ForeignKey("users.id"), index=True)
    amount_usdt: Mapped[Decimal] = mapped_column(Numeric(24, 8))
    network: Mapped[str] = mapped_column(String(16), default="TRC20")
    address: Mapped[str] = mapped_column(String(128))
    status: Mapped[str] = mapped_column(String(16), default="pending", index=True)
    reviewed_by: Mapped[Optional[str]] = mapped_column(String(64))
    review_note: Mapped[Optional[str]] = mapped_column(Text)
    requested_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
    reviewed_at: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True))
    paid_at: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True))
