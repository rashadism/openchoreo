# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging

from collections.abc import Sequence

import httpx
from langchain_core.tools import BaseTool
from langchain_mcp_adapters.client import MultiServerMCPClient, StreamableHttpConnection

from src.config import settings
from src.extensions import ExternalServer

logger = logging.getLogger(__name__)


def _httpx_client_factory(
    headers: dict[str, str] | None = None,
    timeout: httpx.Timeout | None = None,
    auth: httpx.Auth | None = None,
) -> httpx.AsyncClient:
    return httpx.AsyncClient(
        headers=headers,
        timeout=timeout,
        auth=auth,
        verify=not settings.tls_insecure_skip_verify,
    )


class MCPClient:
    def __init__(self, auth: httpx.Auth, external: Sequence[ExternalServer] = ()) -> None:
        obs_connection: StreamableHttpConnection = {
            "transport": "streamable_http",
            "url": settings.observer_mcp_url,
            "httpx_client_factory": _httpx_client_factory,
            "auth": auth,
        }
        oc_connection: StreamableHttpConnection = {
            "transport": "streamable_http",
            "url": settings.openchoreo_mcp_url,
            "httpx_client_factory": _httpx_client_factory,
            "auth": auth,
        }

        self._client = MultiServerMCPClient(
            {
                "observability": obs_connection,
                "openchoreo": oc_connection,
            }
        )
        self._external_names = [server.name for server in external]
        self._external = (
            MultiServerMCPClient(
                {server.name: server.connection for server in external},
                tool_name_prefix=True,
            )
            if external
            else None
        )
        logger.debug("Initialized MCP client with servers: observability, openchoreo")


    async def get_external_tools(self) -> list[BaseTool]:
        if self._external is None:
            return []

        tools: list[BaseTool] = []
        for name in self._external_names:
            try:
                loaded = await self._external.get_tools(server_name=name)
            except Exception as e:
                logger.error("Skipping MCP server %s: %s", name, e, exc_info=True)
                continue
            logger.info("Loaded %d tools from MCP server %s", len(loaded), name)
            tools.extend(loaded)
        return tools

    async def get_tools(self) -> list[BaseTool]:
        try:
            tools = await self._client.get_tools()
        except Exception as e:
            logger.error("Failed to fetch tools from MCP client: %s", e, exc_info=True)
            raise RuntimeError(f"Failed to fetch tools from MCP client: {e}") from e

        return tools
