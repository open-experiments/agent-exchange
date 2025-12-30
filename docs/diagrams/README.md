# Agent Exchange - PlantUML Flow Diagrams

This directory contains PlantUML sequence diagram source files for the Agent Exchange platform flows.

## Diagrams Index

| # | File | Description |
|---|------|-------------|
| 01 | `01-work-submission.puml` | Work submission sequence flow |
| 02 | `02-provider-registration.puml` | Provider registration sequence |
| 03 | `03-bid-submission.puml` | Bid submission sequence |
| 04 | `04-bid-evaluation.puml` | Bid evaluation sequence |
| 05 | `05-contract-award.puml` | Contract award sequence |
| 06 | `06-contract-execution.puml` | Contract execution sequence |
| 07 | `07-settlement.puml` | Settlement process sequence |
| 08 | `08-end-to-end.puml` | Complete end-to-end workflow |
| 09 | `09-event-flow.puml` | Event flow diagram |

## Rendering Diagrams

### Option 1: PlantUML Online Server

Visit [PlantUML Server](http://www.plantuml.com/plantuml) and paste the content of any `.puml` file.

### Option 2: PlantUML CLI

Install PlantUML and render all diagrams:

```bash
# macOS
brew install plantuml graphviz

# Ubuntu/Debian
sudo apt-get install plantuml graphviz

# Generate all PNGs
plantuml docs/diagrams/*.puml

# Generate SVGs
plantuml -tsvg docs/diagrams/*.puml
```

### Option 3: VS Code Extension

Install the [PlantUML extension](https://marketplace.visualstudio.com/items?itemName=jebbs.plantuml) for VS Code:

1. Install extension: `ext install jebbs.plantuml`
2. Open any `.puml` file
3. Press `Alt+D` to preview
4. Right-click to export

### Option 4: Docker

```bash
# Pull PlantUML server
docker pull plantuml/plantuml-server

# Run server
docker run -d -p 8080:8080 plantuml/plantuml-server:jetty

# Access at http://localhost:8080
```

## Related Documentation

- [Architecture Flows (ASCII)](../ARCHITECTURE_FLOWS.md) - ASCII art versions
- [Architecture Flows (Mermaid)](../ARCHITECTURE_FLOWS_MERMAID.md) - Mermaid versions
