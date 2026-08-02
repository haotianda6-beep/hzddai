import hashlib
import unittest
from decimal import Decimal

from sqlalchemy import create_engine
from sqlalchemy.orm import sessionmaker

from app.commission import record_deposit, reverse_deposit
from app.models import Base, PartnerRole, QualificationReferral, User, WithdrawalRequest
from app.partner import (
    assign_role,
    request_studio,
    request_withdrawal,
    review_studio_request,
    review_withdrawal,
)
from app.platform_users import prune_missing_users


class RebateV2Test(unittest.TestCase):
    def setUp(self):
        engine = create_engine("sqlite:///:memory:")
        Base.metadata.create_all(engine)
        self.db = sessionmaker(engine, expire_on_commit=False)()

    def tearDown(self):
        self.db.close()

    def user(self, uid, parent=None, role="retail"):
        row = User(external_uid=uid, nickname=uid, parent_id=parent.id if parent else None, role=role)
        self.db.add(row)
        self.db.flush()
        return row

    def deposit(self, user, amount="100", ref=None):
        row, duplicate = record_deposit(
            self.db, user, Decimal(amount), ref or f"deposit:{user.external_uid}:{amount}", "test"
        )
        self.db.flush()
        return row, duplicate

    def tron_address(self, payload=b"test-wallet"):
        alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
        raw = b"\x41" + hashlib.sha256(payload).digest()[:20]
        raw += hashlib.sha256(hashlib.sha256(raw).digest()).digest()[:4]
        number = int.from_bytes(raw, "big")
        encoded = ""
        while number:
            number, index = divmod(number, 58)
            encoded = alphabet[index] + encoded
        return "1" * (len(raw) - len(raw.lstrip(b"\0"))) + encoded

    def test_longest_chain_uses_20_30_20_differential(self):
        branch = self.user("branch", role=PartnerRole.BRANCH.value)
        studio = self.user("studio", branch, PartnerRole.STUDIO.value)
        ib = self.user("ib", studio, PartnerRole.IB.value)
        retail = self.user("retail", ib)
        self.deposit(retail)
        self.assertEqual(Decimal(branch.rebate_balance_usdt), Decimal("20"))
        self.assertEqual(Decimal(studio.rebate_balance_usdt), Decimal("30"))
        self.assertEqual(Decimal(ib.rebate_balance_usdt), Decimal("20"))

    def test_tiny_commission_never_exceeds_seventy_percent_cap(self):
        branch = self.user("branch", role="branch")
        studio = self.user("studio", branch, "studio")
        ib = self.user("ib", studio, "ib")
        retail = self.user("retail", ib)
        self.deposit(retail, amount="0.00000003", ref="tiny-cap")
        total = sum(
            Decimal(user.rebate_balance_usdt)
            for user in (branch, studio, ib)
        )
        self.assertEqual(total, Decimal("0.00000002"))

    def test_upper_ib_is_skipped_but_studio_and_branch_continue(self):
        branch = self.user("branch", role="branch")
        studio = self.user("studio", branch, "studio")
        upper_ib = self.user("upper-ib", studio, "ib")
        lower_ib = self.user("lower-ib", upper_ib, "ib")
        retail = self.user("retail", lower_ib)
        self.deposit(retail)
        self.assertEqual(Decimal(lower_ib.rebate_balance_usdt), Decimal("20"))
        self.assertEqual(Decimal(upper_ib.rebate_balance_usdt), Decimal("0"))
        self.assertEqual(Decimal(studio.rebate_balance_usdt), Decimal("30"))
        self.assertEqual(Decimal(branch.rebate_balance_usdt), Decimal("20"))

    def test_ib_qualification_has_no_backpay(self):
        studio = self.user("studio", role="studio")
        candidate = self.user("candidate", studio)
        invitees = [self.user(f"child-{i}", candidate) for i in range(6)]
        for i, child in enumerate(invitees[:5]):
            self.deposit(child, ref=f"qualify:{i}")
        self.assertEqual(candidate.role, "ib")
        self.assertEqual(Decimal(candidate.rebate_balance_usdt), Decimal("0"))
        self.assertEqual(Decimal(studio.rebate_balance_usdt), Decimal("250"))
        self.deposit(invitees[5], ref="after-upgrade")
        self.assertEqual(Decimal(candidate.rebate_balance_usdt), Decimal("20"))
        self.assertEqual(Decimal(studio.rebate_balance_usdt), Decimal("280"))

    def test_idempotency_and_reversal_clawback(self):
        ib = self.user("ib", role="ib")
        retail = self.user("retail", ib)
        original, duplicate = self.deposit(retail, ref="same-ref")
        _, duplicate_again = self.deposit(retail, ref="same-ref")
        self.assertFalse(duplicate)
        self.assertTrue(duplicate_again)
        self.assertEqual(Decimal(ib.rebate_balance_usdt), Decimal("20"))
        reverse_deposit(self.db, original, Decimal("50"), "reverse:1")
        self.assertEqual(Decimal(ib.rebate_balance_usdt), Decimal("10"))

    def test_idempotency_rejects_conflicting_payloads(self):
        ib = self.user("ib", role="ib")
        retail = self.user("retail", ib)
        original, _ = self.deposit(retail, ref="same-ref")
        with self.assertRaisesRegex(ValueError, "流水号冲突"):
            self.deposit(retail, amount="999", ref="same-ref")
        with self.assertRaisesRegex(ValueError, "流水号冲突"):
            reverse_deposit(self.db, original, Decimal("10"), "same-ref")
        reversal, _ = reverse_deposit(self.db, original, Decimal("10"), "reverse-ref")
        with self.assertRaisesRegex(ValueError, "流水号冲突"):
            reverse_deposit(self.db, original, Decimal("20"), "reverse-ref")
        self.assertEqual(reversal.event_type, "reversal")

    def test_partial_reversals_finish_at_exact_original_commission(self):
        ib = self.user("ib", role="ib")
        retail = self.user("retail", ib)
        original, _ = self.deposit(retail, amount="0.00000009", ref="tiny-deposit")
        for index in range(3):
            reverse_deposit(self.db, original, Decimal("0.00000003"), f"tiny-reverse:{index}")
        self.assertEqual(Decimal(ib.rebate_balance_usdt), Decimal("0"))

    def test_qualification_reversal_revokes_automatic_ib(self):
        candidate = self.user("candidate")
        invitees = [self.user(f"child-{index}", candidate) for index in range(5)]
        events = [self.deposit(child, ref=f"qualify:{index}")[0] for index, child in enumerate(invitees)]
        self.assertEqual(candidate.role, "ib")
        reverse_deposit(self.db, events[-1], Decimal("100"), "qualify-reversal")
        self.assertEqual(candidate.role, "retail")
        self.assertEqual(
            self.db.query(QualificationReferral)
            .filter(QualificationReferral.candidate_user_id == candidate.id)
            .count(),
            4,
        )

    def test_withdrawal_freeze_approve_paid(self):
        ib = self.user("ib", role="ib")
        ib.rebate_balance_usdt = Decimal("120")
        row = request_withdrawal(self.db, ib, Decimal("100"), self.tron_address())
        self.assertEqual(Decimal(ib.rebate_balance_usdt), Decimal("20"))
        self.assertEqual(Decimal(ib.frozen_balance_usdt), Decimal("100"))
        review_withdrawal(self.db, row, "approve", "admin")
        review_withdrawal(self.db, row, "paid", "admin")
        self.assertEqual(row.status, "paid")
        self.assertEqual(Decimal(ib.frozen_balance_usdt), Decimal("0"))

    def test_withdrawal_reject_restores_balance_and_rejects_invalid_address(self):
        ib = self.user("ib", role="ib")
        ib.rebate_balance_usdt = Decimal("120")
        with self.assertRaisesRegex(ValueError, "TRC20"):
            request_withdrawal(self.db, ib, Decimal("100"), "X" * 34)
        row = request_withdrawal(self.db, ib, Decimal("100"), self.tron_address(b"reject"))
        review_withdrawal(self.db, row, "reject", "admin")
        self.assertEqual(Decimal(ib.rebate_balance_usdt), Decimal("120"))
        self.assertEqual(Decimal(ib.frozen_balance_usdt), Decimal("0"))

    def test_branch_can_only_nominate_own_umbrella(self):
        branch = self.user("branch", role="branch")
        own = self.user("own", branch)
        outside = self.user("outside")
        row = request_studio(self.db, branch, own)
        self.assertEqual(row.status, "pending")
        with self.assertRaises(ValueError):
            request_studio(self.db, branch, outside)
        assign_role(self.db, own, "studio", "admin")
        self.assertEqual(own.role, "studio")

    def test_studio_review_revalidates_branch_and_nested_studios(self):
        branch = self.user("branch", role="branch")
        candidate = self.user("candidate", branch)
        request = request_studio(self.db, branch, candidate)
        assign_role(self.db, branch, "retail", "admin")
        with self.assertRaisesRegex(ValueError, "分公司"):
            review_studio_request(self.db, request, True, "admin")
        self.assertEqual(request.status, "pending")

        other_branch = self.user("other-branch", role="branch")
        ancestor = self.user("ancestor", other_branch)
        self.user("existing-studio", ancestor, "studio")
        with self.assertRaisesRegex(ValueError, "工作室"):
            request_studio(self.db, other_branch, ancestor)

    def test_prune_removes_identity_but_keeps_anonymous_financial_ledger(self):
        ib = self.user("ib", role="ib")
        retail = self.user("retail", ib)
        self.deposit(retail, ref="before-delete")
        self.assertEqual(prune_missing_users(self.db, {"ib"}), 1)
        self.db.flush()
        from app.models import CommissionLedger, DepositEvent

        event = self.db.query(DepositEvent).one()
        commission = self.db.query(CommissionLedger).one()
        self.assertIsNone(event.user_id)
        self.assertIsNone(commission.source_user_id)
        self.assertEqual(commission.recipient_user_id, ib.id)

    def test_prune_keeps_user_with_unsettled_withdrawal(self):
        ib = self.user("ib", role="ib")
        ib.rebate_balance_usdt = Decimal("100")
        row = request_withdrawal(self.db, ib, Decimal("100"), self.tron_address(b"pending"))
        self.assertEqual(prune_missing_users(self.db, set()), 0)
        self.assertIsNotNone(self.db.get(User, ib.id))
        self.assertIsNotNone(self.db.get(WithdrawalRequest, row.id))
        review_withdrawal(self.db, row, "approve", "admin")
        review_withdrawal(self.db, row, "paid", "admin")
        self.assertEqual(prune_missing_users(self.db, set()), 1)
        self.assertIsNone(self.db.get(User, ib.id))

    def test_non_finite_money_is_rejected(self):
        retail = self.user("retail")
        for value in ("NaN", "Infinity", "-Infinity"):
            with self.assertRaisesRegex(ValueError, "金额格式"):
                self.deposit(retail, amount=value, ref=f"invalid:{value}")


if __name__ == "__main__":
    unittest.main()
