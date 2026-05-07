# Architecture

This document provides visual diagrams of the DataRobot CLI's key architectural patterns and data flows.

## Data flow: dr start

```mermaid
graph LR
    A["User runs<br/>dr start"] --> B["Cobra parses<br/>arguments"]
    B --> C["Load config<br/>from drconfig.yaml"]
    C --> D{"Authenticated?"}
    D -->|No| E["Show auth prompt<br/>TUI Model"]
    E --> F["OAuth flow<br/>get token"]
    F --> G["Save token<br/>to config"]
    D -->|Yes| H["Show template<br/>selection TUI"]
    H --> I["User selects<br/>template"]
    I --> J["Fetch template<br/>via drapi.Client"]
    J --> K["Discover env vars<br/>from spec"]
    K --> L["Prompt for values<br/>TUI Model"]
    L --> M["Validate config<br/>vs spec"]
    M --> N["Copy template<br/>to project"]
    N --> O["Generate .env<br/>file"]
    O --> P["Send telemetry<br/>event"]
    P --> Q["Complete"]
    
    style A fill:#e1f5ff
    style Q fill:#c8e6c9
    style F fill:#ffccbc
    style J fill:#e8f5e9
    style P fill:#f1f8e9
```

### As a sequence diagram

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant Config
    participant Auth as OAuth/Auth
    participant TUI as TUI Model
    participant API as drapi.Client
    participant EnvBuilder
    participant FileSystem
    participant Telemetry

    User->>CLI: dr start
    CLI->>Config: Load drconfig.yaml
    Config-->>CLI: Config data
    
    CLI->>CLI: Check if authenticated
    
    alt Not authenticated
        CLI->>TUI: Show auth prompt
        TUI->>User: Request auth
        User->>TUI: Grant permission
        TUI->>Auth: OAuth flow
        Auth-->>TUI: Token
        TUI->>Config: Save token
        Config-->>TUI: Saved
    end
    
    CLI->>TUI: Show template selection
    TUI->>User: Display templates
    User->>TUI: Select template
    
    TUI->>API: Fetch template spec
    API-->>TUI: Template data
    
    TUI->>EnvBuilder: Discover env vars
    EnvBuilder-->>TUI: Required/optional vars
    
    TUI->>User: Prompt for values
    User->>TUI: Enter values
    TUI->>EnvBuilder: Validate config
    EnvBuilder-->>TUI: Validation result
    
    TUI->>FileSystem: Copy template files
    FileSystem-->>TUI: Copied
    
    TUI->>FileSystem: Generate .env file
    FileSystem-->>TUI: Generated
    
    TUI->>Telemetry: Track event
    Telemetry-->>TUI: Ack
    
    TUI-->>User: Complete
```

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
