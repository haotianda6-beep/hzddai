"""应用配置"""
from pathlib import Path

from pydantic_settings import BaseSettings, SettingsConfigDict

# 项目根目录（含 agent-rebate.env），避免单独 `uvicorn` 未 export 环境变量时用错默认口令
_ROOT = Path(__file__).resolve().parent.parent


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=(
            _ROOT / ".env",
            _ROOT / "agent-rebate.env",
        ),
        env_file_encoding="utf-8",
        extra="ignore",
        # 避免环境里空的 PLATFORM_SYNC_SECRET= 覆盖 agent-rebate.env 里的值
        env_ignore_empty=True,
    )

    database_url: str = "sqlite:///./agent_rebate.db"
    admin_token: str = "change-me-in-production"
    secret_key: str = "change-me"
    # 平台 webhook 同步用户（POST /api/platform/user-sync）必填请求头 X-Platform-Secret
    platform_sync_secret: str = ""
    # True：启动时尝试删除 external_uid 以 demo- 开头的历史演示用户及关联流水
    purge_demo_users_on_startup: bool = True
    # POST /api/platform/user-sync 单次 batch 模式最多条数（防止误传超大列表）
    max_platform_user_sync_batch: int = 2000
    # 可选：管理端一键导入 hzddai 用户时的 SQLite 路径（默认 nofx data.db）
    hzddai_sqlite_path: str = "/root/hzddai/data/data.db"


settings = Settings()


def validate_runtime_settings() -> None:
    if settings.admin_token.strip() in {"", "change-me-in-production"}:
        raise RuntimeError("ADMIN_TOKEN未配置安全值")
    if not settings.platform_sync_secret.strip():
        raise RuntimeError("PLATFORM_SYNC_SECRET未配置")
