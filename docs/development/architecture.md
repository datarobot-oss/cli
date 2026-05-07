# Architecture

This document provides visual diagrams of the DataRobot CLI's key architectural patterns and data flows.

## Plugin loading

```mermaid
sequenceDiagram
    participant CLI as CLI Init
    participant PM as plugin.Manager
    participant FS as File System
    participant REG as Registry API
    participant Cache as Plugin Cache
    participant Load as Plugin Binary
    participant Root as Root Command

    CLI->>PM: Load all plugins
    PM->>FS: Check ~/.config/datarobot/plugins/
    FS-->>PM: List local plugin files
    
    PM->>REG: Fetch remote plugin definitions
    REG-->>PM: Plugin metadata + URLs
    
    PM->>Cache: Check if cached locally
    
    alt Plugin not cached
        PM->>REG: Download binary
        REG-->>PM: Plugin binary
        PM->>Cache: Store in cache
    else Plugin cached
        Cache-->>PM: Return cached binary
    end
    
    PM->>Load: Load plugin executable
    Load-->>PM: Plugin commands
    
    PM->>Root: Register as subcommands
    Root-->>CLI: Ready to execute
```

## Configuration flow

```mermaid
graph TD
    CMD["Command Layer<br/>(Cobra)"]
    
    CMD -->|Read persistent| CONFIG["internal/config<br/>(Viper)"]
    CMD -->|Read transient<br/>from flags| FLAGS["cmd.Flags()"]
    
    CONFIG -->|Env var override| VIPER["viper instance<br/>(via viperx)"]
    FLAGS -->|Never bind<br/>to viper| VIPER
    
    VIPER -->|Persist key| WRITE["config.UpdateConfigFile<br/>(PersistableKeys only)"]
    
    WRITE -->|Update YAML| FILE["~/.config/datarobot<br/>/drconfig.yaml"]
    
    style CMD fill:#fff3e0
    style CONFIG fill:#fce4ec
    style WRITE fill:#f3e5f5
    style FILE fill:#e1f5ff
```

## Model-View-Cmd pattern for CLI commands

```mermaid
graph LR
    USER["User invokes<br/>dr &lt;cmd&gt;"]
    
    USER --> CMD
    
    subgraph "Command Layer"
        CMD["cmd.RunE<br/>(entry point)"]
    end
    
    subgraph "Infrastructure"
        PARSE["Parse flags<br/>from cobra"]
        VALIDATE["Validate inputs"]
    end
    
    subgraph "Domain Model"
        MODEL["Business logic<br/>(state, calculations,<br/>decisions)"]
    end
    
    subgraph "View Layer"
        TUI["TUI interactive model<br/>(Bubble Tea)"]
        TEXT["Formatted text output<br/>(Bubble Tea)"]
        JSON["JSON output"]
        CSV["CSV output"]
    end
    
    subgraph "External"
        API["drapi.Client<br/>(API calls)"]
        CONFIG["config<br/>(persistence)"]
        STATE["state.yaml<br/>(state persistence)"]
    end
    
    CMD --> PARSE
    PARSE --> VALIDATE
    VALIDATE --> MODEL
    
    MODEL --> TUI
    MODEL --> TEXT
    MODEL --> JSON
    MODEL --> CSV
    MODEL --> API
    MODEL --> CONFIG
    MODEL --> STATE
    
    TUI -->|user input| MODEL
    
    style USER fill:#e0f2f1
    style CMD fill:#fff3e0
    style MODEL fill:#f3e5f5
    style TUI fill:#ede7f6
    style TEXT fill:#e1f5ff
    style API fill:#e8f5e9
    style CONFIG fill:#fce4ec
    style STATE fill:#fce4ec
```

## Build and release

```mermaid
graph LR
    SRC["Source Code"]
    
    SRC --> TASK["task build<br/>(Task runner)"]
    
    TASK --> LDFLAG["Apply ldflags<br/>(version, commit)"]
    LDFLAG --> GR["GoReleaser<br/>(goreleaser.yaml)"]
    
    GR --> BIN["dist/dr<br/>(binary)"]
    GR --> CHECKSUM["checksums.txt"]
    GR --> RELEASE["GitHub Release"]
    
    style SRC fill:#e1f5ff
    style TASK fill:#fff3e0
    style GR fill:#ede7f6
    style BIN fill:#c8e6c9
    style RELEASE fill:#ffccbc
```

## Next steps

- [Project Structure](structure.md) - Detailed directory layout
- [Building & Development](building.md) - Build process and testing
- [Configuration Management](configuration.md) - Config files and flags
- [Authentication](authentication.md) - OAuth flow details
- [Plugins](plugins.md) - Plugin development guide
