# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0


class OpenchoreoTools:
    LIST_ENVIRONMENTS = "list_environments"
    LIST_NAMESPACES = "list_namespaces"
    LIST_PROJECTS = "list_projects"
    LIST_COMPONENTS = "list_components"


class ObservabilityTools:
    GET_TRACES = "get_traces"
    GET_NAMESPACE_LOGS = "get_namespace_logs"
    GET_PROJECT_LOGS = "get_project_logs"
    GET_COMPONENT_LOGS = "get_component_logs"
    GET_COMPONENT_RESOURCE_METRICS = "get_component_resource_metrics"


obs_tools = ObservabilityTools()
oc_tools = OpenchoreoTools()

# Tools available to each agent type
RCA_AGENT_TOOLS = {
    obs_tools.GET_TRACES,
    obs_tools.GET_COMPONENT_LOGS,
    obs_tools.GET_PROJECT_LOGS,
    obs_tools.GET_COMPONENT_RESOURCE_METRICS,
    oc_tools.LIST_ENVIRONMENTS,
    oc_tools.LIST_NAMESPACES,
    oc_tools.LIST_PROJECTS,
    oc_tools.LIST_COMPONENTS,
}

CHAT_AGENT_TOOLS = {
    obs_tools.GET_TRACES,
    obs_tools.GET_COMPONENT_LOGS,
    obs_tools.GET_PROJECT_LOGS,
    obs_tools.GET_COMPONENT_RESOURCE_METRICS,
    oc_tools.LIST_ENVIRONMENTS,
    oc_tools.LIST_NAMESPACES,
    oc_tools.LIST_PROJECTS,
    oc_tools.LIST_COMPONENTS,
}


class OpenchoreoLabels:
    COMPONENT_UID = "openchoreo.dev/component-uid"
    COMPONENT_UIDS = "openchoreo.dev/component-uids"
    ENVIRONMENT_UID = "openchoreo.dev/environment-uid"
    PROJECT_UID = "openchoreo.dev/project-uid"


class Templates:
    # Agent prompts
    RCA_AGENT_PROMPT = "prompts/rca_agent_prompt.j2"
    CHAT_AGENT_PROMPT = "prompts/chat_agent_prompt.j2"
    # API
    RCA_REQUEST = "api/rca_request.j2"
    # Transformers
    COMPONENT_LOGS_TRANSFORMER = "middleware/component_logs.j2"
    PROJECT_LOGS_TRANSFORMER = "middleware/project_logs.j2"
    METRICS_TRANSFORMER = "middleware/metrics.j2"
    TRACES_TRANSFORMER = "middleware/traces.j2"


oc_labels = OpenchoreoLabels()
templates = Templates()
