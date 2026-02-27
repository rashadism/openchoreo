# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from src.clients.backend.opensearch_backend import get_report_backend
from src.clients.backend.report_backend import ReportBackend

__all__ = [
    "ReportBackend",
    "get_report_backend",
]
