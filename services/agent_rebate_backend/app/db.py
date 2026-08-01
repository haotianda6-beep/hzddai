from __future__ import annotations

from sqlalchemy import create_engine, event, inspect
from sqlalchemy.orm import sessionmaker

from app.config import settings
from app.models import Base


engine = create_engine(
    settings.database_url,
    echo=False,
    future=True,
    connect_args={"timeout": 30} if settings.database_url.startswith("sqlite") else {},
)
SessionLocal = sessionmaker(bind=engine, autoflush=False, autocommit=False, future=True)


if settings.database_url.startswith("sqlite"):

    @event.listens_for(engine, "connect")
    def set_sqlite_pragmas(dbapi_connection, _):
        cursor = dbapi_connection.cursor()
        cursor.execute("PRAGMA foreign_keys=ON")
        cursor.execute("PRAGMA journal_mode=WAL")
        cursor.execute("PRAGMA busy_timeout=30000")
        cursor.close()


def init_db() -> None:
    old_tables = {
        "commission_records",
        "recharge_transactions",
        "vip_tier_config",
        "weekly_vip5_dividend_pools",
    }
    found = old_tables.intersection(inspect(engine).get_table_names())
    if found:
        raise RuntimeError("检测到旧邀请返佣数据库，请先执行全新体系初始化")
    Base.metadata.create_all(bind=engine)
