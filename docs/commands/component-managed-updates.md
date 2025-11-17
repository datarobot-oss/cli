# Managed Component Updates

This feature allows you to manage component updates and additions with preconfigured default answers, following copier's `data_file` semantics with support for multiple repositories.

## Overview

When working with copier-based components, you often need to provide the same answers repeatedly. This feature provides three ways to streamline this:

1. **CLI `--data` arguments**: Pass answers directly on the command line
2. **Data file**: Store default answers in a YAML file (follows copier's `data_file` convention)
3. **Automatic discovery**: Data files are automatically discovered in priority order

## CLI Usage

### Component Add

Add a component with specific answers:

```bash
dr component add git@github.com:datarobot-community/af-component-agent.git \
  --data base_answers_file=.datarobot/answers/base.yml \
  --data llm_answers_file=.datarobot/answers/llm-llm.yml \
  --data use_low_code_interface=false
```

Add a component using a specific data file:

```bash
dr component add git@github.com:datarobot-community/af-component-agent.git \
  --data-file .datarobot/.copier-answers-defaults.yaml
```

### Component Update

Update a component with specific answers:

```bash
dr component update .datarobot/answers/agent-writer_agent.yml \
  --data base_answers_file=.datarobot/answers/base.yml \
  --data llm_answers_file=.datarobot/answers/llm-llm.yml \
  --data use_low_code_interface=false
```

Update using a specific data file:

```bash
dr component update .datarobot/answers/agent-writer_agent.yml \
  --data-file my-custom-defaults.yaml
```

This is equivalent to the copier command:

```bash
copier update -a .datarobot/answers/agent-writer_agent.yml \
  --data base_answers_file=.datarobot/answers/base.yml \
  --data llm_answers_file=.datarobot/answers/llm-llm.yml \
  --data use_low_code_interface=false
```

## Data File

### Naming and Location

Following copier's `data_file` convention, the default filename is `.copier-answers-defaults.yaml`.

**Discovery Priority Order:**

1. **Explicit path** via `--data-file` flag (highest priority)
2. **Repository root**: `.datarobot/.copier-answers-defaults.yaml`
3. **User config directory**: `~/.config/datarobot/.copier-answers-defaults.yaml`
4. **Legacy location**: `~/.config/datarobot/component-defaults.yaml` (backward compatibility)

### Format

```yaml
defaults:
  git@github.com:datarobot-community/af-component-agent.git:
    base_answers_file: .datarobot/answers/base.yml
    llm_answers_file: .datarobot/answers/llm-llm.yml
    use_low_code_interface: false
```

### How It Works

1. When you run `dr component add` or `dr component update`, the CLI:
   - Reads the copier answers file to determine the component's repository URL
   - Looks up defaults for that repository in `component-defaults.yaml`
   - Applies those defaults automatically

2. CLI `--data` arguments always take precedence over configured defaults

3. Any questions not covered by defaults or CLI arguments will be prompted interactively (unless you use copier's `-A` flag, which is included in `dr component update`)

### Recommended Setup

**For team-wide defaults** (committed to the repository):

Create `.datarobot/.copier-answers-defaults.yaml` in your repository root:

```yaml
defaults:
  git@github.com:datarobot-community/af-component-agent.git:
    base_answers_file: .datarobot/answers/base.yml
    llm_answers_file: .datarobot/answers/llm-llm.yml
    use_low_code_interface: false
```

**For personal defaults** (not committed):

Create `~/.config/datarobot/.copier-answers-defaults.yaml`:

```yaml
defaults:
  git@github.com:datarobot-community/af-component-agent.git:
    agent_template_framework: langgraph
```

### Example Workflow

With the repository data file shown above, running:

```bash
dr component add git@github.com:datarobot-community/af-component-agent.git
```

Is equivalent to:

```bash
dr component add git@github.com:datarobot-community/af-component-agent.git \
  --data base_answers_file=.datarobot/answers/base.yml \
  --data llm_answers_file=.datarobot/answers/llm-llm.yml \
  --data use_low_code_interface=false
```

If you want to override a default:

```bash
dr component add git@github.com:datarobot-community/af-component-agent.git \
  --data use_low_code_interface=true
```

This will use the configured defaults for `base_answers_file` and `llm_answers_file`, but override `use_low_code_interface` to `true`.

You can also use a different data file temporarily:

```bash
dr component add git@github.com:datarobot-community/af-component-agent.git \
  --data-file /path/to/custom-defaults.yaml
```

## Benefits

1. **Team Consistency**: Repository-level defaults ensure the same answers across team members
2. **Speed**: Skip repetitive prompts for common answers
3. **Flexibility**: Override defaults on a per-command basis when needed
4. **Personal Customization**: Personal defaults in home directory for individual preferences
5. **Compatibility**: Follows copier's `data_file` convention for familiarity
6. **Version Control Friendly**: Repository defaults can be committed, personal defaults stay local

## Data Types

The data file and `--data` arguments support all copier question types:

- **Strings**: `key: value` or `--data key=value`
- **Booleans**: `key: true` or `--data use_feature=false`
- **Integers**: `key: 42` or `--data count=42`
- **Floats**: `key: 3.14` or `--data height=1.83`
- **Lists** (for multiselect): `key: [item1, item2]` or `--data items=[1, 2, 3]`
- **Objects/Maps**: `key: {nested: value}` or `--data config={key: value}`
- **Null**: `key: null` or `--data optional=null`
- **YAML/JSON** complex types: Any valid YAML structure in the data file

The data file uses standard YAML syntax. CLI `--data` arguments are parsed according to the question's type defined in the copier template.
