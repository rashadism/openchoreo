# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import copy
import json
import logging
import re
import uuid
from collections import defaultdict
from collections.abc import AsyncIterator
from dataclasses import dataclass
from typing import Any

from src.agent.helpers import AlertScope, _parse_mcp_response
from src.auth.bearer import BearerTokenAuth
from src.logging_config import request_id_context
from src.models.remediation_result import ActionStatus, RemediationAction

logger = logging.getLogger(__name__)

# Maps camelCase override field names to the snake_case MCP parameter names
OVERRIDE_PARAM_MAP: dict[str, str] = {
    "workloadOverrides": "workload_overrides",
    "componentTypeEnvOverrides": "component_type_env_overrides",
    "traitOverrides": "trait_overrides",
}

# Regex to match array selector segments like env[key=POSTGRES_DSN]
_ARRAY_SELECTOR_RE = re.compile(r"^(\w+)\[(\w+)=([^\]]+)\]$")


def _emit(event: dict[str, Any]) -> str:
    return json.dumps(event) + "\n"


def parse_resource_ref(resource: str) -> tuple[str, str]:
    parts = resource.strip().split(None, 1)
    if len(parts) != 2:
        raise ValueError(f"Invalid resource reference: '{resource}'. Expected 'Kind name'.")
    return parts[0], parts[1]


def parse_field_path(field_path: str) -> tuple[str, list[str | tuple[str, str, str]]]:
    segments = field_path.split(".")
    if len(segments) < 3 or segments[0] != "spec":
        raise ValueError(
            f"Invalid field_path: '{field_path}'. Expected 'spec.<overrideCategory>.<path...>'."
        )

    override_key = segments[1]  # e.g. "workloadOverrides"
    if override_key not in OVERRIDE_PARAM_MAP:
        raise ValueError(
            f"Unsupported override category: '{override_key}'. "
            f"Supported: {list(OVERRIDE_PARAM_MAP.keys())}"
        )

    path_segments: list[str | tuple[str, str, str]] = []
    for seg in segments[2:]:
        m = _ARRAY_SELECTOR_RE.match(seg)
        if m:
            path_segments.append((m.group(1), m.group(2), m.group(3)))
        else:
            path_segments.append(seg)

    return override_key, path_segments


def apply_change(
    override: dict[str, Any],
    segments: list[str | tuple[str, str, str]],
    value: str,
) -> None:
    current = override
    for i, seg in enumerate(segments):
        is_last = i == len(segments) - 1

        if isinstance(seg, tuple):
            array_field, match_key, match_val = seg
            arr = current.setdefault(array_field, [])
            # Try exact match first, then fall back to common identifier fields
            # (e.g., field path says name=X but existing data uses key=X)
            entry = next((item for item in arr if item.get(match_key) == match_val), None)
            if entry is None:
                for alt_key in ("key", "name"):
                    if alt_key != match_key:
                        entry = next((item for item in arr if item.get(alt_key) == match_val), None)
                        if entry is not None:
                            break
            if entry is None:
                entry = {match_key: match_val}
                arr.append(entry)
            if is_last:
                raise ValueError(
                    f"Array selector '{array_field}[{match_key}={match_val}]' "
                    "cannot be the final segment — need a field after it."
                )
            current = entry
        else:
            if is_last:
                current[seg] = value
            else:
                current = current.setdefault(seg, {})


@dataclass
class BindingInfo:
    component_name: str
    release_name: str
    raw: dict[str, Any]


async def _build_binding_lookup(
    list_components_tool: Any,
    list_rb_tool: Any,
    scope: AlertScope,
) -> dict[str, BindingInfo]:
    raw_components = await list_components_tool.ainvoke(
        {
            "namespace_name": scope.namespace_name,
            "project_name": scope.project_name,
        }
    )
    components = _parse_mcp_response(raw_components, "components")

    lookup: dict[str, BindingInfo] = {}
    for component in components:
        comp_name = component["name"]
        raw_bindings = await list_rb_tool.ainvoke(
            {
                "namespace_name": scope.namespace_name,
                "project_name": scope.project_name,
                "environment_name": scope.environment_name,
                "component_name": comp_name,
            }
        )
        bindings = _parse_mcp_response(raw_bindings, "bindings")
        for binding in bindings:
            lookup[binding["name"]] = BindingInfo(
                component_name=comp_name,
                release_name=binding["releaseName"],
                raw=binding,
            )

    return lookup


async def stream_patch(
    patch_context: Any,
    token: str,
    scope: AlertScope,
) -> AsyncIterator[str]:
    request_id_context.set(f"patch_{uuid.uuid4().hex[:12]}")

    try:
        actions = [RemediationAction(**a) for a in patch_context.actions]
    except Exception as e:
        logger.error("Failed to parse patch actions: %s", e, exc_info=True)
        yield _emit({"type": "error", "message": f"Invalid patch actions: {e}"})
        return

    # Pair each action with its client-provided index for UI tracking
    indexed_actions: list[tuple[int, RemediationAction]] = []
    for i, (raw, parsed) in enumerate(zip(patch_context.actions, actions, strict=True)):
        if parsed.status == ActionStatus.REVISED and parsed.changes:
            idx = raw.get("index", i)
            indexed_actions.append((idx, parsed))

    if not indexed_actions:
        yield _emit({"type": "patch_started"})
        yield _emit({"type": "patch_done", "summary": "No revised actions to apply"})
        return

    # Initialize MCP client and get the tools we need
    try:
        from src.clients import MCPClient

        mcp_client = MCPClient(auth=BearerTokenAuth(token))
        all_tools = await mcp_client.get_tools()
        tools_by_name = {t.name: t for t in all_tools}

        required = ["list_components", "list_release_bindings", "patch_release_binding"]
        missing = [n for n in required if n not in tools_by_name]
        if missing:
            yield _emit(
                {
                    "type": "error",
                    "message": f"Required MCP tools not available: {', '.join(missing)}",
                }
            )
            return

        list_components_tool = tools_by_name["list_components"]
        list_rb_tool = tools_by_name["list_release_bindings"]
        patch_rb_tool = tools_by_name["patch_release_binding"]
    except Exception as e:
        logger.error("Failed to initialize MCP client: %s", e, exc_info=True)
        yield _emit({"type": "error", "message": f"Failed to connect to MCP server: {e}"})
        return

    # Build binding_name → component_name lookup
    try:
        binding_lookup = await _build_binding_lookup(list_components_tool, list_rb_tool, scope)
    except Exception as e:
        logger.error("Failed to build binding lookup: %s", e, exc_info=True)
        yield _emit({"type": "error", "message": f"Failed to resolve release bindings: {e}"})
        return

    yield _emit({"type": "patch_started"})

    success_count = 0
    total = len(indexed_actions)

    for action_index, action in indexed_actions:
        action_desc = action.description
        yield _emit(
            {
                "type": "patch_progress",
                "index": action_index,
                "action": action_desc,
                "status": "applying",
            }
        )

        try:
            # Group changes by (resource_kind, resource_name) so we can batch
            # multiple changes to the same binding into a single patch call.
            # Store raw change info — we apply after fetching existing overrides.
            grouped: dict[tuple[str, str], list[tuple[str, str, list, str]]] = defaultdict(list)

            for change in action.changes:
                kind, name = parse_resource_ref(change.resource)
                if kind != "ReleaseBinding":
                    yield _emit(
                        {
                            "type": "patch_result",
                            "index": action_index,
                            "action": action_desc,
                            "status": "skipped",
                            "details": f"Unsupported resource kind: '{kind}'",
                        }
                    )
                    continue

                override_key, path_segments = parse_field_path(change.field_path)
                param_name = OVERRIDE_PARAM_MAP[override_key]
                grouped[(kind, name)].append(
                    (override_key, param_name, path_segments, change.value)
                )

            if not grouped:
                continue

            for (kind, name), changes in grouped.items():
                info = binding_lookup.get(name)
                if not info:
                    yield _emit(
                        {
                            "type": "patch_result",
                            "index": action_index,
                            "action": action_desc,
                            "status": "failed",
                            "details": f"No component found for release binding '{name}'",
                        }
                    )
                    continue

                # Deep-copy existing overrides from the binding, then apply changes
                # on top. patch_release_binding fully replaces each override param,
                # so we must send the complete modified override back.
                overrides_by_param: dict[str, dict[str, Any]] = {}
                for override_key, param_name, path_segments, value in changes:
                    if param_name not in overrides_by_param:
                        existing = info.raw.get(override_key, {})
                        overrides_by_param[param_name] = copy.deepcopy(existing)
                    apply_change(overrides_by_param[param_name], path_segments, value)

                patch_args: dict[str, Any] = {
                    "namespace_name": scope.namespace_name,
                    "project_name": scope.project_name,
                    "environment": scope.environment_name,
                    "component_name": info.component_name,
                    "binding_name": name,
                    "release_name": info.release_name,
                }
                patch_args.update(overrides_by_param)

                logger.debug("patch_release_binding args: %s", patch_args)
                await patch_rb_tool.ainvoke(patch_args)

                yield _emit(
                    {
                        "type": "patch_result",
                        "index": action_index,
                        "action": action_desc,
                        "status": "success",
                        "details": f"Applied to {kind} {name}",
                    }
                )
                success_count += 1

        except Exception as e:
            logger.error("Failed to apply action '%s': %s", action_desc, e, exc_info=True)
            yield _emit(
                {
                    "type": "patch_result",
                    "index": action_index,
                    "action": action_desc,
                    "status": "failed",
                    "details": str(e),
                }
            )

    yield _emit(
        {
            "type": "patch_done",
            "summary": f"Applied {success_count}/{total} fixes",
        }
    )
