<!--
  ~ Copyright 2025 DataRobot, Inc. and its affiliates.
  ~
  ~ Licensed under the Apache License, Version 2.0 (the "License");
  ~ you may not use this file except in compliance with the License.
  ~ You may obtain a copy of the License at
  ~
  ~     http://www.apache.org/licenses/LICENSE-2.0
  ~
  ~ Unless required by applicable law or agreed to in writing, software
  ~ distributed under the License is distributed on an "AS IS" BASIS,
  ~ WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  ~ See the License for the specific language governing permissions and
  ~ limitations under the License.
-->

<!--
  ~ Copyright 2025 DataRobot, Inc. and its affiliates.
  ~ All rights reserved.
  ~ DataRobot, Inc. Confidential.
  ~ This is unpublished proprietary source code of DataRobot, Inc.
  ~ and its affiliates.
  ~ The copyright notice above does not evidence any actual or intended
  ~ publication of such source code.
-->

# Command Line Interface (CLI) for DataRobot

Modules in this folder implement various command line commands and subcommands for interacting with DataRobot services.

There should be a one-to-one mapping between commands/subcommands and modules in this folder. Each module typically contains the logic for parsing command line arguments, handling user input, and invoking the appropriate functions from internal or tui packages to perform the desired operations.
