#!/bin/bash

touch .env

dr help
dr help run
dr version
dr completion bash | unbuffer -p > completion_bash.sh
# TODO: Confirm we have some expected content in bash file using grep most likely
# cat completion_bash.sh
dr run

# Pass URL value to command
echo "https://www.example.com" | unbuffer -p dr auth setURL
ls -la
# TODO: check created config file to confirm we've set the auth URL (maybe use grep or yq?)
# cat .config/datarobot/drconfig.yaml

# Hit enter key
echo -ne '\n' | unbuffer -p dr dotenv setup
# TODO: check .env file to confirm we've done some setup (maybe use grep or yq?)
echo "cat -u .env"
cat -u .env

echo "cat -u CHANGELOG.md"
cat -u CHANGELOG.md

# Hit `q` key
echo -ne 'q' | unbuffer -p dr templates setup
