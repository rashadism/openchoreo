# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from typing import Annotated, Any, Literal

from fastapi import APIRouter, Depends, HTTPException, Query
from pydantic import ConfigDict, Field

from src.auth import require_authn, require_reports_authz
from src.auth.authz_models import SubjectContext
from src.clients import get_report_backend
from src.helpers import resolve_project_scope, validate_time_range
from src.models import BaseModel

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/api/v1/rca-agent/reports", tags=["RCA Reports"])


class RCAReportSummary(BaseModel):
    alert_id: str = Field(alias="alertId")
    report_id: str = Field(alias="reportId")
    timestamp: str
    summary: str | None = None
    status: str

    model_config = ConfigDict(populate_by_name=True)


class RCAReportsResponse(BaseModel):
    reports: list[RCAReportSummary]
    total_count: int = Field(alias="totalCount")

    model_config = ConfigDict(populate_by_name=True)


class RCAReportDetailed(BaseModel):
    alert_id: str = Field(alias="alertId")
    report_id: str = Field(alias="reportId")
    timestamp: str
    status: str
    report: dict[str, Any] | None = None

    model_config = ConfigDict(populate_by_name=True)


@router.get(
    "/",
    response_model=RCAReportsResponse,
    response_model_by_alias=True,
)
async def list_rca_reports(
    project: str,
    environment: str,
    namespace: str,
    start_time: Annotated[str, Query(alias="startTime")],
    end_time: Annotated[str, Query(alias="endTime")],
    _auth: Annotated[SubjectContext, Depends(require_authn)],
    _authz: Annotated[SubjectContext, Depends(require_reports_authz)],
    limit: Annotated[int, Query(ge=1, le=10000)] = 100,
    sort: Literal["asc", "desc"] = "desc",
    status: Literal["pending", "completed", "failed"] | None = None,
):
    validate_time_range(start_time, end_time)

    scope = await resolve_project_scope(
        namespace=namespace,
        project=project,
        environment=environment,
    )

    report_backend = get_report_backend()
    result = await report_backend.list_rca_reports(
        project_uid=scope.project_uid,
        environment_uid=scope.environment_uid,
        start_time=start_time,
        end_time=end_time,
        status=status,
        limit=limit,
        sort=sort,
    )

    return RCAReportsResponse(
        reports=[RCAReportSummary(**r) for r in result["reports"]],
        totalCount=result["totalCount"],
    )


@router.get(
    "/{report_id}",
    response_model=RCAReportDetailed,
    response_model_by_alias=True,
)
async def get_rca_report(
    report_id: str,
    _auth: Annotated[SubjectContext, Depends(require_authn)],
    _authz: Annotated[SubjectContext, Depends(require_reports_authz)],
):
    report_backend = get_report_backend()
    result = await report_backend.get_rca_report(report_id)

    if not result:
        raise HTTPException(status_code=404, detail="Report not found")

    return RCAReportDetailed(
        alertId=result["alertId"],
        reportId=result["reportId"],
        timestamp=result["@timestamp"],
        status=result["status"],
        report=result.get("report"),
    )
