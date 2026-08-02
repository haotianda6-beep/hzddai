import unittest

from fastapi import HTTPException
from starlette.requests import Request

from app.config import settings, validate_runtime_settings
from app.deps import require_admin, require_platform


def request(headers=None):
    raw = [(key.lower().encode(), value.encode()) for key, value in (headers or {}).items()]
    return Request({"type": "http", "headers": raw})


class SecurityTest(unittest.TestCase):
    def setUp(self):
        self.admin_token = settings.admin_token
        self.platform_secret = settings.platform_sync_secret
        settings.admin_token = "admin-test-secret"
        settings.platform_sync_secret = "platform-test-secret"

    def tearDown(self):
        settings.admin_token = self.admin_token
        settings.platform_sync_secret = self.platform_secret

    def test_platform_secret_is_required(self):
        with self.assertRaises(HTTPException) as denied:
            require_platform(request())
        self.assertEqual(denied.exception.status_code, 401)
        require_platform(request({"X-Platform-Secret": "platform-test-secret"}))

    def test_admin_accepts_only_admin_or_platform_secret(self):
        with self.assertRaises(HTTPException) as denied:
            require_admin(request({"X-Admin-Token": "wrong"}))
        self.assertEqual(denied.exception.status_code, 401)
        require_admin(request({"X-Admin-Token": "admin-test-secret"}))
        require_admin(request({"X-Platform-Secret": "platform-test-secret"}))
        require_admin(request({"Cookie": "admin_token=admin-test-secret"}))

    def test_insecure_runtime_defaults_are_rejected(self):
        settings.admin_token = "change-me-in-production"
        with self.assertRaisesRegex(RuntimeError, "ADMIN_TOKEN"):
            validate_runtime_settings()
        settings.admin_token = "admin-test-secret"
        settings.platform_sync_secret = ""
        with self.assertRaisesRegex(RuntimeError, "PLATFORM_SYNC_SECRET"):
            validate_runtime_settings()


if __name__ == "__main__":
    unittest.main()
