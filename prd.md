Codemap

Overview

Codemap is a repository onboarding and codebase learning system.

Its purpose is not to search code.

Its purpose is not to generate documentation.

Its purpose is not to explain every file.

Its purpose is to answer one question:

«If I have never seen this repository before, what should I read first, what should I read next, and which parts of the code actually matter?»

Codemap analyzes an entire repository, determines the optimal reading order, identifies the most important files, extracts the most important code regions, hides low-value implementation details, and guides the developer through the codebase in a structured way.

The system acts like an experienced engineer who has already studied the repository and is showing a new developer exactly where to look.

---

Problem

When developers open an unfamiliar repository they are immediately overwhelmed.

Large repositories may contain:

- Hundreds of files
- Thousands of functions
- Tens of thousands of lines of code

The most common questions are:

- Where do I start?
- Which file should I read first?
- Which files are critical?
- Which files can be ignored initially?
- Which code is architecture?
- Which code is implementation detail?
- Which code is boilerplate?
- How do I understand the system without reading everything?

Traditional tools do not solve this problem.

GitHub shows files.

Editors show files.

Documentation may be outdated.

Developers are forced to manually discover the architecture.

Codemap exists to solve this onboarding problem.

---

Core Philosophy

Most developers do not need to read all code.

Most developers need to understand:

- System entrypoints
- Core abstractions
- Execution flow
- Business logic
- Important interfaces
- Important data structures

A large percentage of repository code is noise during onboarding:

- Logging
- Metrics
- Validation
- Error wrapping
- Utility functions
- Generated code
- Test code
- Configuration plumbing

Codemap identifies the signal and suppresses the noise.

---

High-Level Workflow

Repository
    ↓
Scan Files
    ↓
Parse Source Code
    ↓
Extract Symbols
    ↓
Build Relationships
    ↓
Rank Importance
    ↓
Generate Reading Order
    ↓
Extract Important Code
    ↓
Present Guided Learning Experience

---

What Codemap Produces

1. Recommended Reading Order

Example:

1. main.go
2. router.go
3. auth_handler.go
4. auth_service.go
5. user_repository.go

The user should immediately know where to start.

---

2. File Importance

Every file is analyzed and ranked according to its contribution to understanding the system.

Example:

main.go
Reason:
Application entrypoint

router.go
Reason:
Defines request flow

auth_service.go
Reason:
Contains core authentication logic

---

3. Important Code Extraction

This is the most important feature in the project.

Instead of showing entire files, Codemap extracts the sections required to understand the system.

Example:

Visible:

func main() {
    cfg := LoadConfig()

    db := ConnectDatabase()

    router := BuildRouter()

    StartServer(router)
}

Hidden:

logger.Debug(...)
metrics.Record(...)
if err != nil { ... }
...

The user should see the important code immediately.

---

4. Hidden Code Sections

Non-essential code should be collapsed.

Example:

▼ 187 lines hidden

The user can expand the section if desired.

Important code is visible.

Less important code is folded.

Nothing is removed.

Everything remains accessible.

---

5. Contextual Explanations

Every important file should contain:

Why this file matters

Key symbols

What the developer should learn from it

Example:

This file starts the application and wires together
all major components.

---

Current Technical Architecture

Scanner

Responsible for:

- Walking repository structure
- Discovering source files
- Ignoring vendor folders
- Ignoring generated code
- Ignoring irrelevant directories

---

Parser

Responsible for:

- Struct extraction
- Interface extraction
- Function extraction
- Method extraction
- Import extraction

---

Symbol System

Core repository entities:

Struct
Interface
Function
Method

Symbols become the foundation for analysis.

---

Reference Graph

Relationships between symbols:

Uses
Constructs
Returns
Implements
Embeds
References

Used to understand architecture.

---

Ranking Engine

Responsible for determining:

- Important files
- Important symbols
- Reading order

The ranking is optimized for human learning rather than graph correctness.

The goal is:

What should a developer learn first?

not:

What has the most references?

---

CLI Product

Primary interface today:

codemap /path/to/repository

Output:

Recommended Reading Order

1. main.go
2. router.go
3. auth_handler.go
...

This is currently the fastest way to validate the learning engine.

---

Final Product Vision

The final product is a guided code-reading platform.

A developer opens a repository and receives a curated learning experience.

---

Left Side

Reading order.

Example:

1. main.go
2. router.go
3. auth_handler.go
4. auth_service.go

---

Center

Selected file.

The user sees:

Why read this file

Then:

Important code

Then:

Hidden code sections

The center panel is primarily code.

Not dashboards.

Not charts.

Not metrics.

Code.

---

Right Side

Context.

Example:

Purpose

Why it matters

Key symbols

Dependencies introduced

---

Explicit Non-Goals

Codemap is NOT trying to become:

- GitHub
- VS Code
- Cursor
- Windsurf
- Documentation generator
- Architecture diagram tool
- Static analysis platform

The primary goal remains:

Help developers understand unfamiliar repositories faster.

---

Future AI Features

AI is not the core product.

The learning engine is the core product.

AI will be added on top of curated knowledge.

Examples:

Explain this file

Explain this symbol

Explain this hidden code

Explain why this code is not important

Summarize this execution flow

The AI should operate on selected context.

Not the entire repository.

---

Most Important Product Principle

If a developer opens a repository with 50 files, Codemap should make them feel:

I know exactly which file to read first.

I know why I am reading it.

I know which lines matter.

I know which lines I can ignore for now.

Everything in the project should support that objective.
