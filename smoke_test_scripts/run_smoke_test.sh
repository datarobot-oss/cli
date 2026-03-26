#!/bin/bash

# Be sure to get DR_API_TOKEN from args
args=("$@")
DR_API_TOKEN=${args[0]}
if [[ -z "$DR_API_TOKEN" ]]; then
  echo "❌ The variable 'DR_API_TOKEN' must be supplied as arg."
  exit 1
fi

export TERM="dumb"

# Timing helpers
SCRIPT_START=$(date +%s)
TEST_TIMINGS=""

start_timer() {
    TEST_NAME="$1"
    TEST_START=$(date +%s)
    echo ""
    echo "▶ $TEST_NAME"
}

stop_timer() {
    local elapsed=$(( $(date +%s) - TEST_START ))
    echo "  ⏱  ${TEST_NAME}: ${elapsed}s"
    TEST_TIMINGS="${TEST_TIMINGS}  ${elapsed}s\t${TEST_NAME}\n"
}

# Used throughout testing
testing_url="https://app.datarobot.com"

# Determine if we can access URL
wget -q --spider "$testing_url"
if [ $? -eq 0 ]; then
    url_accessible=1
else
    url_accessible=0
fi

# Using `DATAROBOT_CLI_CONFIG` to be sure we can save/update config file in GitHub Action runners
testing_dr_cli_config_dir="$(pwd)/.config/datarobot/"
mkdir -p "$testing_dr_cli_config_dir"
export DATAROBOT_CLI_CONFIG="${testing_dr_cli_config_dir}drconfig.yaml"
touch "$DATAROBOT_CLI_CONFIG"
cat "$(pwd)/smoke_test_scripts/assets/example_config.yaml" > "$DATAROBOT_CLI_CONFIG"

# Set API token in our ephemeral config file
yq -i ".token = \"$DR_API_TOKEN\"" "$DATAROBOT_CLI_CONFIG"

start_timer "Check datarobot alias"
if command -v datarobot >/dev/null 2>&1 || [ -x "$HOME/.local/bin/datarobot" ]; then
    echo "✅ 'datarobot' alias is available."
else
    echo "❌ 'datarobot' alias not found - expected symlink at $HOME/.local/bin/datarobot"
    echo "   Debug: Contents of $HOME/.local/bin/:"
    ls -la "$HOME/.local/bin/" 2>&1 || echo "   Directory does not exist"
    exit 1
fi
stop_timer

start_timer "dr help output"
header_copy="Build AI Applications Faster"
has_header=$(dr help | grep "${header_copy}")
if [[ -n "$has_header" ]]; then
    echo "✅ Help command returned expected content."
else
    echo "❌ Help command did not return expected content - missing header copy: ${header_copy}"
    exit 1
fi
stop_timer

start_timer "datarobot alias help output"
has_header_alias=$(datarobot help | grep "${header_copy}" 2>/dev/null || "$HOME/.local/bin/datarobot" help | grep "${header_copy}")
if [[ -n "$has_header_alias" ]]; then
    echo "✅ 'datarobot' alias returned expected content."
else
    echo "❌ 'datarobot' alias did not return expected content."
    exit 1
fi
stop_timer

start_timer "dr self version --format=json"
has_version_key=$(dr self version --format=json | yq eval 'has("version")')
if [[ "$has_version_key" == "true" ]]; then
    echo "✅ Version command returned expected 'version' key in json output."
else
    echo "❌ Version command did not return expected 'version' key in json output."
    exit 1
fi
stop_timer

start_timer "dr self completion bash"
dr self completion bash > completion_bash.sh
function_check=$(cat completion_bash.sh | grep __start_dr\()
if [[ -n "$function_check" ]]; then
  echo "✅ Assertion passed: We have expected completion_bash.sh file."
  rm completion_bash.sh
else
  echo "❌ Assertion failed: We don't have expected completion_bash.sh file w/ expected function: __start_dr()."
  cat completion_bash.sh
  exit 1
fi
stop_timer

start_timer "Completion install/uninstall (expect)"
expect ./smoke_test_scripts/expect_completion.exp
stop_timer

start_timer "dr run usage message"
if [ -f ".env" ]; then
    usage_message="No Taskfiles found in child directories."
else
    usage_message="You don't seem to be in a DataRobot Template directory."
fi
has_message=$(dr run 2>&1 | grep "${usage_message}")
if [[ -n "$has_message" ]]; then
    echo "✅ Run command returned expected content."
else
    echo "❌ Run command did not return expected content - missing informative message: ${usage_message}"
    exit 1
fi
stop_timer

start_timer "dr auth setURL (expect)"
expect ./smoke_test_scripts/expect_auth_setURL.exp "$DATAROBOT_CLI_CONFIG"
auth_endpoint_check=$(cat "$DATAROBOT_CLI_CONFIG" | grep endpoint | grep "${testing_url}/api/v2")
if [[ -n "$auth_endpoint_check" ]]; then
  echo "✅ Assertion passed: We have expected expected 'endpoint' auth URL value in config."
  echo "Value: $auth_endpoint_check"
else
  echo "❌ Assertion failed: We don't have expected 'endpoint' auth URL value."
  echo "${DATAROBOT_CLI_CONFIG} contents:"
  cat "$DATAROBOT_CLI_CONFIG"
  exit 1
fi
stop_timer

start_timer "dr auth login (expect)"
expect ./smoke_test_scripts/expect_auth_login.exp
stop_timer

start_timer "dr templates setup + dotenv"
if [ "$url_accessible" -eq 0 ]; then
  echo "ℹ️ URL (${testing_url}) is not accessible so skipping 'dr templates setup' test."
  stop_timer
else
  expect ./smoke_test_scripts/expect_templates_setup.exp
  DIRECTORY="./talk-to-my-docs-agents"
  if [ -d "$DIRECTORY" ]; then
    echo "✅ Directory ($DIRECTORY) exists."
  else
    echo "❌ Directory ($DIRECTORY) does not exist."
    exit 1
  fi
  cd "$DIRECTORY"

  # Validate the SESSION_SECRET_KEY was auto-generated with a non-empty value
  # Extract the value between quotes after SESSION_SECRET_KEY=
  session_secret_key_value=$(sed -n 's/^SESSION_SECRET_KEY="\([^"]*\)".*/\1/p' .env)
  if [[ -n "$session_secret_key_value" ]]; then
    echo "✅ Assertion passed: SESSION_SECRET_KEY has auto-generated value in .env file."
  else
    echo "❌ Assertion failed: SESSION_SECRET_KEY is empty or missing in .env file."
    cat .env
    exit 1
  fi

  # Now test dr dotenv setup within the template directory
  echo "Testing dr dotenv setup within template directory..."

  # Run dotenv setup - it should prompt for existing variables including DATAROBOT_ENDPOINT
  # The expect script will accept defaults for all variables
  export DATAROBOT_ENDPOINT="${testing_url}"
  expect ../smoke_test_scripts/expect_dotenv_setup.exp "."

  # Validate DATAROBOT_ENDPOINT exists in .env (it should already be there from template)
  endpoint_check=$(cat .env | grep "DATAROBOT_ENDPOINT")
  if [[ -n "$endpoint_check" ]]; then
    echo "✅ Assertion passed: dr dotenv setup preserved DATAROBOT_ENDPOINT in template .env file."
    echo "Value: $endpoint_check"
  else
    echo "❌ Assertion failed: DATAROBOT_ENDPOINT not found in .env file."
    cat .env
    cd ..
    rm -rf "$DIRECTORY"
    exit 1
  fi

  # Now delete directory to clean up
  cd ..
  rm -rf "$DIRECTORY"
  stop_timer
fi

# Print timing summary
TOTAL_ELAPSED=$(( $(date +%s) - SCRIPT_START ))
echo ""
echo "══════════════════════════════════════"
echo "  Smoke Test Timing Summary"
echo "══════════════════════════════════════"
printf "$TEST_TIMINGS"
echo "──────────────────────────────────────"
echo "  Total: ${TOTAL_ELAPSED}s"
echo "══════════════════════════════════════"
