# Codemap

Codemap is a repository onboarding and codebase learning system. It acts like an experienced engineer who has already studied the repository and is showing a new developer exactly where to look.

## Overview

When developers open an unfamiliar repository they are immediately overwhelmed by hundreds of files and tens of thousands of lines of code. Traditional tools, like editors or version control platforms, simply present files without context, forcing developers to manually discover the architecture.

Codemap solves this onboarding problem by answering one crucial question: "If I have never seen this repository before, what should I read first, what should I read next, and which parts of the code actually matter?"

## Core Philosophy

Most developers do not need to read all code to understand a repository. They need to understand:

* System entrypoints
* Core abstractions
* Execution flow
* Business logic
* Important interfaces and data structures

A large percentage of repository code is noise during onboarding (logging, metrics, validation, error wrapping, generated code). Codemap identifies the signal and suppresses the noise.

## Features

* **Recommended Reading Order**
  Provides a clear, ordered list of files to start reading, eliminating confusion about where to begin.

* **File Importance and Context**
  Every file is analyzed and ranked according to its contribution to understanding the system, complete with contextual explanations of why the file matters and what key symbols it introduces.

* **Important Code Extraction**
  Instead of showing entire files, Codemap extracts the exact sections required to understand the system. Critical code is kept visible, while non-essential boilerplate is folded and hidden by default.

* **Guided Learning Experience**
  A dedicated web interface providing a curated code-reading platform, dividing the experience into the reading order, the selected code slice, and contextual learning details.

## Architecture

* **Scanner:** Walks the repository structure, discovering source files while ignoring vendor folders and irrelevant directories.
* **Parser:** Extracts structs, interfaces, functions, methods, and imports from source code.
* **Symbol System and Reference Graph:** Builds relationships between symbols (Uses, Constructs, Returns, Implements, Embeds) to understand the architecture.
* **Ranking Engine:** Determines the importance of files and symbols optimized for human learning.

## Getting Started

Codemap operates with a Go-based backend and a React frontend. The project provides a `Makefile` to simplify installation, building, and running.

1. **Install Dependencies** (Go modules and npm packages):
   ```bash
   make install-deps
   ```

2. **Build the Project** (compiles the Go binary to `bin/codemap` and builds the frontend):
   ```bash
   make build
   ```

3. **Start the Application** (runs both the backend API and frontend concurrently):
   ```bash
   make start
   ```

4. **Clean Build Artifacts** (optional):
   ```bash
   make clean
   ```

## Explicit Non-Goals

Codemap is not trying to become a replacement for GitHub, an IDE like VS Code, a documentation generator, an architecture diagram tool, or a generic static analysis platform. The primary objective is exclusively to help developers understand unfamiliar repositories faster.
