# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from functools import lru_cache
from typing import Any

from langchain.chat_models import init_chat_model
from langchain_core.language_models import BaseChatModel

from src.config import settings

logger = logging.getLogger(__name__)

# SummarizationMiddleware's fractional trigger raises without max_input_tokens,
# which provider packages only ship for models released before the pinned version.
_FALLBACK_MAX_INPUT_TOKENS = 128_000


def _ensure_max_input_tokens(model: BaseChatModel, model_name: str) -> BaseChatModel:
    # Fill only when absent; profile= at construction would override real data.
    profile = model.profile or {}
    if isinstance(profile.get("max_input_tokens"), int):
        return model
    logger.warning(
        "No model profile for %s; assuming max_input_tokens=%d. Upgrade the provider "
        "package to pick up the real limit.",
        model_name,
        _FALLBACK_MAX_INPUT_TOKENS,
    )
    model.profile = {**profile, "max_input_tokens": _FALLBACK_MAX_INPUT_TOKENS}
    return model


@lru_cache
def get_model(
    model_name: str = settings.llm_name,
    api_key: str = settings.llm_api_key,
    **kwargs: Any,
) -> BaseChatModel:
    # Route through an OpenAI-compatible proxy (the ai-gateway-agentgateway
    # module) when configured; the real provider key then lives at the gateway
    # so api_key may be a placeholder. Forward base_url only when set to leave
    # the direct-to-provider path unchanged.
    if settings.finops_agent_llm_base_url and "base_url" not in kwargs:
        kwargs["base_url"] = settings.finops_agent_llm_base_url
    model = init_chat_model(
        model=model_name,
        api_key=api_key,
        max_tokens=settings.finops_llm_max_tokens,
        **kwargs,
    )
    return _ensure_max_input_tokens(model, model_name)
