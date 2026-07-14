import { HttpStatus, Injectable } from "@nestjs/common";
import {
  ConnectionMode,
  type EnvironmentTemplateView,
} from "@gpu-rental/contracts";

import { DomainException } from "../common/domain-exception";

const ENVIRONMENT_TEMPLATES: EnvironmentTemplateView[] = [
  {
    id: "pytorch-jupyter",
    name: "PyTorch + JupyterLab",
    description: "CUDA-enabled PyTorch workspace with JupyterLab access.",
    image: "pytorch/pytorch:2.7.1-cuda12.8-cudnn9-runtime",
    category: "training",
    connectionModes: [
      ConnectionMode.Jupyter,
      ConnectionMode.Ssh,
      ConnectionMode.WebTerminal,
    ],
  },
  {
    id: "cuda-development",
    name: "CUDA Development",
    description: "Minimal CUDA development image for custom workloads.",
    image: "nvidia/cuda:12.8.1-devel-ubuntu24.04",
    category: "development",
    connectionModes: [ConnectionMode.Ssh, ConnectionMode.WebTerminal],
  },
  {
    id: "vllm-inference",
    name: "vLLM Inference",
    description: "OpenAI-compatible model serving environment based on vLLM.",
    image: "vllm/vllm-openai:v0.10.0",
    category: "inference",
    connectionModes: [ConnectionMode.Ssh, ConnectionMode.WebTerminal],
  },
];

@Injectable()
export class EnvironmentTemplatesService {
  list(): EnvironmentTemplateView[] {
    return ENVIRONMENT_TEMPLATES.map((template) => ({ ...template }));
  }

  getById(id = ENVIRONMENT_TEMPLATES[0]!.id): EnvironmentTemplateView {
    const template = ENVIRONMENT_TEMPLATES.find(
      (candidate) => candidate.id === id,
    );
    if (!template) {
      throw new DomainException(
        "ENVIRONMENT_TEMPLATE_NOT_FOUND",
        "The environment template was not found",
        HttpStatus.NOT_FOUND,
      );
    }
    return { ...template };
  }
}
