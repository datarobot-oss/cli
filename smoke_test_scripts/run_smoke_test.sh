#!/bin/bash

set -euo pipefail

# Helper functions
error() {
  echo "❌ $1"
  exit 1
}

success() {
  echo "✅ $1"
}

info() {
  echo "ℹ️ $1"
}

# Be sure to get DR_API_TOKEN from args
args=("$@")
DR_API_TOKEN=${args[0]}
if [[ -z "$DR_API_TOKEN" ]]; then
  error "The variable 'DR_API_TOKEN' must be supplied as arg."
fi

export TERM="dumb"

# Used throughout testing
testing_url="https://app.datarobot.com"

# Using `DATAROBOT_CLI_CONFIG` to be sure we can save/update config file in GitHub Action runners
testing_dr_cli_config_dir="$(pwd)/.config/datarobot/"
mkdir -p "$testing_dr_cli_config_dir"
export DATAROBOT_CLI_CONFIG="${testing_dr_cli_config_dir}drconfig.yaml"

cleanup() {
  info "Cleaning up..."
  rm -rf "$testing_dr_cli_config_dir"
  rm -f completion_bash.sh
}

trap cleanup EXIT

# Determine if we can access URL
wget -q --spider "$testing_url"
if [ $? -eq 0 ]; then
    url_accessible=1
else
    url_accessible=0
fi

touch "$DATAROBOT_CLI_CONFIG"
cat "$(pwd)/smoke_test_scripts/assets/example_config.yaml" > "$DATAROBOT_CLI_CONFIG"

# Set API token in our ephemeral config file
yq -i ".token = \"$DR_API_TOKEN\"" "$DATAROBOT_CLI_CONFIG"

# Check we have expected help output (checking for header content)
header_copy="Build AI Applications Faster"
has_header=$(dr help | grep "${header_copy}")
if [[ -n "$has_header" ]]; then
    success "Help command returned expected content."
else
    error "Help command did not return expected content - missing header copy: ${header_copy}"
fi

# Check that JSON output of version command has expected `version` key
has_version_key=$(dr self version --format=json | yq eval 'has("version")')
if [[ "$has_version_key" == "true" ]]; then
    success "Version command returned expected 'version' key in json output."
else
    error "Version command did not return expected 'version' key in json output."
fi

dr self completion bash > completion_bash.sh
# Check if we have the file with expected __start_dr() function
function_check=$(cat completion_bash.sh | grep __start_dr\()
if [[ -n "$function_check" ]]; then
  success "Assertion passed: We have expected completion_bash.sh file."
else
  # Print completion_bash.sh (if it exists) to aid in debugging if needed
  cat completion_bash.sh
  error "Assertion failed: We don't have expected completion_bash.sh file w/ expected function: __start_dr()."
fi

# Test completion install/uninstall interactively
info "Testing completion install/uninstall..."
expect ./smoke_test_scripts/expect_completion.exp

# Check we have expected usage message output
if [ -f ".env" ]; then
    usage_message="No Taskfiles found in child directories."
else
    usage_message="You don't seem to be in a DataRobot Template directory."
fi

info "Testing dr run command..."
# Use 2>&1 to stderr to stdout
has_message=$(dr run 2>&1 | grep "${usage_message}")
if [[ -n "$has_message" ]]; then
    success "Run command returned expected content."
else
    error "Run command did not return expected content - missing informative message: ${usage_message}"
fi

# Use expect to run commands as user and we expect to update auth URL config value using `dr auth setURL`
# The expect script "hits" the `y` key for "yes", then `https://app.datarobot.com`
expect ./smoke_test_scripts/expect_auth_setURL.exp "$DATAROBOT_CLI_CONFIG"

# Check if we have the auth URL correctly set
auth_endpoint_check=$(cat "$DATAROBOT_CLI_CONFIG" | grep endpoint | grep "${testing_url}/api/v2")
if [[ -n "$auth_endpoint_check" ]]; then
  success "Assertion passed: We have expected 'endpoint' auth URL value in config."
  info "Value: $auth_endpoint_check"
else
  # Print ~/.config/datarobot/drconfig.yaml (if it exists) to aid in debugging if needed
  info "${DATAROBOT_CLI_CONFIG} contents:"
  cat "$DATAROBOT_CLI_CONFIG"
  error "Assertion failed: We don't have expected 'endpoint' auth URL value."
fi

# Test `dr auth login` and we should have the value shown in output:
# `https://app.datarobot.com/account/developer-tools?cliRedirect=true`
info "Testing dr auth login..."
expect ./smoke_test_scripts/expect_auth_login.exp

# Test templates - Confirm expect script has cloned TTMDocs and that .env has expected value
if [ "$url_accessible" -eq 0 ]; then
  info "URL (${testing_url}) is not accessible so skipping 'dr templates setup' test."
else
  info "Testing dr templates setup..."
  expect ./smoke_test_scripts/expect_templates_setup.exp
  testing_session_secret_key="TESTING_SESSION_SECRET_KEY"
  DIRECTORY="./talk-to-my-docs-agents"
  if [ -d "$DIRECTORY" ]; then
    success "Directory ($DIRECTORY) exists."
  else
    error "Directory ($DIRECTORY) does not exist."
  fi
  cd "$DIRECTORY"

  # Validate the SESSION_SECRET_KEY set during templates setup
  session_secret_key_check=$(cat .env | grep "SESSION_SECRET_KEY=\"${testing_session_secret_key}\"")
  if [[ -n "$session_secret_key_check" ]]; then
    success "Assertion passed: We have expected SESSION_SECRET_KEY in created .env file."
  else
    cat .env
    error "Assertion failed: We don't have expected SESSION_SECRET_KEY value in created .env file."
  fi

  # Now test dr dotenv setup within the template directory
  info "Testing dr dotenv setup within template directory..."

  # Run dotenv setup - it should prompt for existing variables including DATAROBOT_ENDPOINT
  # The expect script will accept defaults for all variables
  export DATAROBOT_ENDPOINT="${testing_url}"
  expect ../smoke_test_scripts/expect_dotenv_setup.exp "."

  # Validate DATAROBOT_ENDPOINT exists in .env (it should already be there from template)
  endpoint_check=$(cat .env | grep "DATAROBOT_ENDPOINT")
  if [[ -n "$endpoint_check" ]]; then
    success "Assertion passed: dr dotenv setup preserved DATAROBOT_ENDPOINT in template .env file."
    info "Value: $endpoint_check"
  else
    cat .env
    cd ..
    rm -rf "$DIRECTORY"
    error "Assertion failed: DATAROBOT_ENDPOINT not found in .env file."
  fi

  # Now delete directory to clean up
  cd ..
  rm -rf "$DIRECTORY"
fi
