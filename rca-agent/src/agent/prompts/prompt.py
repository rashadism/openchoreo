# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from enum import Enum

from langchain_core.tools import BaseTool

from src.constants import templates
from src.template_manager import render


class Agent(Enum):
    RCA = templates.RCA_AGENT_PROMPT
    CHAT = templates.CHAT_AGENT_PROMPT


def get_prompt(agent: Agent, tools: list[BaseTool]) -> str:
    context = {
        "tools": tools,
        "openchoreo_tools": True,
        "observability_tools": True,
    }
    return render(agent.value, context)
