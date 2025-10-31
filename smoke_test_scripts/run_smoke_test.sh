#!/bin/bash

# Be sure to get DR_API_TOKEN from args
args=("$@")
DR_API_TOKEN=${args[0]}

# Used throughout testing
testing_url="https://app.datarobot.com"

# Using `DATAROBOT_CLI_CONFIG` to be sure we can save/update config file in GitHub Action runners
testing_dr_cli_config_dir="$(pwd)/.config/datarobot/"
mkdir -p $testing_dr_cli_config_dir
export DATAROBOT_CLI_CONFIG="${testing_dr_cli_config_dir}drconfig.yaml"
touch $DATAROBOT_CLI_CONFIG
cat "$(pwd)/smoke_test_scripts/assets/example_config.yaml" > $DATAROBOT_CLI_CONFIG

# Set API token in our ephemeral config file
yq -i ".token = \"$DR_API_TOKEN\"" $DATAROBOT_CLI_CONFIG

dr help
dr help run
dr version

dr completion bash > completion_bash.sh
# Check if we have the file with expected __start_dr() function
function_check=$(cat completion_bash.sh | grep __start_dr\()
if [[ -n "$function_check" ]]; then
  echo "✅ Assertion passed: We have expected completion_bash.sh file."
  # Remove created bash file - especially helpful when running smoke tests locally
  rm completion_bash.sh
else
  echo "❌ Assertion failed: We don't have expected completion_bash.sh file w/ expected function: __start_dr()."
  # Print completion_bash.sh (if it exists) to aid in debugging if needed
  cat completion_bash.sh
  exit 1
fi

dr run

# Use expect to run commands as user and we expect to update auth URL config value using `dr auth setURL`
# The expect script "hits" the `y` key for "yes", then `https://app.datarobot.com`
expect ./smoke_test_scripts/expect_auth_setURL.exp $DATAROBOT_CLI_CONFIG

# Check if we have the auth URL correctly set
auth_endpoint_check=$(cat $DATAROBOT_CLI_CONFIG | grep endpoint | grep ${testing_url}/api/v2)
if [[ -n "$auth_endpoint_check" ]]; then
  echo "✅ Assertion passed: We have expected expected 'endpoint' auth URL value in config."
  echo "Value: $auth_endpoint_check"
else
  echo "❌ Assertion failed: We don't have expected 'endpoint' auth URL value."
  # Print ~/.config/datarobot/drconfig.yaml (if it exists) to aid in debugging if needed
  echo "${DATAROBOT_CLI_CONFIG} contents:"
  cat $DATAROBOT_CLI_CONFIG
  exit 1
fi

# Test `dr auth login` and we should have the value shown in output:
# `https://app.datarobot.com/account/developer-tools?cliRedirect=true`
expect ./smoke_test_scripts/expect_auth_login.exp

# Used to test `dr dotenv setup`
export DATAROBOT_ENDPOINT=${testing_url}

# Use expect to run commands (`dr dotenv setup`) as user and we expect creation of a .env file w/ "https://app.datarobot.com"
# The expect script "hits" the `e` key, then `ctrl-s` and finally `enter` (via carriage return/newline)
expect ./smoke_test_scripts/expect_dotenv_setup.exp
# Check if we have the value correctly set
endpoint_check=$(cat .env | grep DATAROBOT_ENDPOINT=${testing_url}/api/v2)
if [[ -n "$endpoint_check" ]]; then
  echo "✅ Assertion passed: We have expected DATAROBOT_ENDPOINT value (${testing_url}/api/v2) in created .env file."
  echo "Value: $endpoint_check"
else
  echo "❌ Assertion failed: We don't have expected DATAROBOT_ENDPOINT value in created .env file."
  # Print .env (if it exists) to aid in debugging if needed
  cat .env
  exit 1
fi

# Test templates - Confirm expect script has cloned TTMDocs and that .env has expected value
expect ./smoke_test_scripts/expect_templates_setup.exp
testing_session_secret_key="TESTING_SESSION_SECRET_KEY"
DIRECTORY="./talk-to-my-docs-agents"
if [ -d "$DIRECTORY" ]; then
  echo "✅ Directory ($DIRECTORY) exists."
else
  echo "❌ Directory ($DIRECTORY) does not exist."
  exit 1
fi
cd $DIRECTORY
session_secret_key_check=$(cat .env | grep SESSION_SECRET_KEY=${testing_session_secret_key})
if [[ -n "$session_secret_key_check" ]]; then
  echo "✅ Assertion passed: We have expected SESSION_SECRET_KEY in created .env file."
else
  echo "❌ Assertion failed: We don't have expected SESSION_SECRET_KEY value in created .env file."
  # Print .env (if it exists) to aid in debugging if needed
  cat .env
  exit 1
fi
# Now delete directory to clean up
cd ..
rm -rf $DIRECTORY
