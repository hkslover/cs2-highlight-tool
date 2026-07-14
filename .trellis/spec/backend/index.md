# Backend Development Guidelines

> Best practices for backend development in this project.

---

## Overview

This directory contains the project's actual coding conventions, extracted from the real codebase. These guidelines help AI agents write code that matches the team's established patterns.

---

## Guidelines Index

| Guide | Description | Status |
|-------|-------------|--------|
| [Directory Structure](./directory-structure.md) | Module organization and file layout | ✅ Populated |
| [Database Guidelines](./database-guidelines.md) | Config JSON persistence, no SQL database | ✅ Populated |
| [Error Handling](./error-handling.md) | Error wrapping, step-based failure, validation | ✅ Populated |
| [Quality Guidelines](./quality-guidelines.md) | Test patterns, code standards, forbidden patterns | ✅ Populated |
| [Logging Guidelines](./logging-guidelines.md) | Structured logging, step timing, fields conventions | ✅ Populated |
| [Wails Bindings](./wails-bindings.md) | Frontend-facing Go method contracts and storage APIs | ✅ Populated |
| [Produce WebSocket](./producews.md) | Stable loopback listener, HLAE/plugin environment, diagnostics, and reconnect recovery | ✅ Populated |
| [Startup State Machine](./startup-state-machine.md) | Startup self-update gating and component task ordering | ✅ Populated |

---

## How to Use These Guidelines

1. **Before writing code**: Read the relevant guideline for your package
2. **During code review**: Check against patterns documented here
3. **When adding a feature**: Follow the established patterns — match what the codebase *actually does*

## For AI Agents

When dispatched as `trellis-implement` or `trellis-check`, these spec files are auto-injected into your prompt via `implement.jsonl` / `check.jsonl`. Read them before writing code.
