# may - AI Agent Guide

## Project Structure

```
demo/                 Scripts and tools to demo MAY
drivers/aws           Kubernetes Controller to manage Instances on AWS Cloud
drivers/ibm           Kubernetes Controller to manage Instances on IBM Cloud
drivers/incluster     Development only Kubernetes Controller to manage Instances in cluster
may/                  Kubernetes Controller MAY's core logic
```

MAY's core functionalities are implemented under `may`.
MAY is extended by decoupled controllers implemented under the `drivers` folder.
Each Kubernetes Controller in this repo is provided with its own AGENTS.md file.

## Critical Rules

Respect the Critical Rules in the per-component AGENTS.md files too.

### Keep Project Structure
Do not move files around. The CLI expects files in specific locations.
