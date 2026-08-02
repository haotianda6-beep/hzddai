from decimal import Decimal, InvalidOperation

MONEY = Decimal("0.00000001")
RATE = Decimal("0.0001")
MAX_MONEY = Decimal("9999999999999999.99999999")


def money(value: Decimal | str | int | float) -> Decimal:
    try:
        amount = Decimal(str(value))
        if not amount.is_finite():
            raise ValueError("金额格式无效")
        amount = amount.quantize(MONEY)
    except (InvalidOperation, ValueError) as exc:
        raise ValueError("金额格式无效") from exc
    if abs(amount) > MAX_MONEY:
        raise ValueError("金额超出系统范围")
    return amount


def rate(value: Decimal | str | int | float) -> Decimal:
    try:
        result = Decimal(str(value))
        if not result.is_finite():
            raise ValueError("比例格式无效")
        return result.quantize(RATE)
    except (InvalidOperation, ValueError) as exc:
        raise ValueError("比例格式无效") from exc
