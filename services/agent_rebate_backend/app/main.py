from pathlib import Path
import secrets

from fastapi import Depends, FastAPI, Form, HTTPException, Request
from fastapi.responses import HTMLResponse, RedirectResponse
from fastapi.templating import Jinja2Templates

from app.config import settings, validate_runtime_settings
from app.db import init_db
from app.deps import require_admin
from app.routes_admin import router as admin_router
from app.routes_platform import router as platform_router

BASE_DIR = Path(__file__).resolve().parent.parent
templates = Jinja2Templates(directory=str(BASE_DIR / "templates"))
app = FastAPI(title="COMKUN Partner Rebate", version="2.0.1")
app.include_router(platform_router)
app.include_router(admin_router)
app.include_router(admin_router, prefix="/hongzhong", include_in_schema=False)


@app.on_event("startup")
def startup():
    validate_runtime_settings()
    init_db()


@app.get("/health")
def health():
    return {"ok": True, "version": "2.0.1"}


@app.get("/")
def index():
    return RedirectResponse("/hongzhong/login", status_code=302)


@app.get("/hongzhong/login", response_class=HTMLResponse)
def login_page(request: Request):
    return templates.TemplateResponse(request, "admin_login.html", {"error": None})


@app.post("/hongzhong/login")
def login(request: Request, token: str = Form(...)):
    if not token.strip() or not secrets.compare_digest(token.strip(), settings.admin_token):
        return templates.TemplateResponse(
            request, "admin_login.html", {"error": "令牌错误"}, status_code=401
        )
    response = RedirectResponse("/hongzhong/dashboard", status_code=302)
    response.set_cookie(
        "admin_token",
        token.strip(),
        httponly=True,
        secure=True,
        samesite="strict",
        max_age=604800,
    )
    return response


@app.get("/hongzhong/dashboard", response_class=HTMLResponse, dependencies=[Depends(require_admin)])
def dashboard_page(request: Request):
    return templates.TemplateResponse(request, "admin_dashboard.html", {})


@app.api_route(
    "/api/platform/{old_path:path}",
    methods=["GET", "POST", "PUT", "DELETE"],
    include_in_schema=False,
)
def retired_platform_endpoint(old_path: str):
    raise HTTPException(status_code=410, detail=f"旧邀请返佣接口已根除: {old_path}")


@app.api_route(
    "/api/{old_path:path}",
    methods=["GET", "POST", "PUT", "DELETE"],
    include_in_schema=False,
)
def retired_customer_endpoint(old_path: str):
    raise HTTPException(status_code=410, detail=f"旧客户返佣接口已根除: {old_path}")
