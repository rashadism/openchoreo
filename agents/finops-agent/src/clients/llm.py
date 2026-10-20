# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from functools import lru_cache
from typing import Any

from langchain.chat_models import init_chat_model
from langchain_core.language_models import BaseChatModel

from src.config import settings


def _is_openai_model(model_name: str) -> bool:
    # init_chat_model defaults a bare (no "provider:" prefix) model name to OpenAI.
    provider = model_name.split(":", 1)[0].lower() if ":" in model_name else "openai"
    return provider == "openai"


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
    # OpenAI's reasoning models reject function tools on /v1/chat/completions,
    # requiring /v1/responses instead. Only safe to force when talking
    # directly to OpenAI; with a base_url override we don't know the real
    # backend (e.g. ai-gateway-agentgateway can route elsewhere), so leave it.
    if (
        not settings.finops_agent_llm_base_url
        and _is_openai_model(model_name)
        and "use_responses_api" not in kwargs
    ):
        kwargs["use_responses_api"] = True
    return init_chat_model(
        model=model_name,
        api_key=api_key,
        max_tokens=settings.finops_llm_max_tokens,
        **kwargs,
    )
