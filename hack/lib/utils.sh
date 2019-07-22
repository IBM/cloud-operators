
#!/bin/bash
#
# Copyright 2019 IBM Corporation
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

COLOR_RESET="\e[00m"
COLOR_GREEN="\e[1;32m"
COLOR_RED="\e[00;31m"

BOLD=$(tput bold)
NORMAL=$(tput sgr0)

CHECKMARK="${COLOR_GREEN}✔${COLOR_RESET}"
CROSSMARK="${COLOR_RED}✗${COLOR_RESET}"

LASTCASE=0
EXIT_CODE=0

# print header in bold
function u::header() {
    echo ""
    echo ${BOLD}${1}${NORMAL}
}

# print test suite name
function u::testsuite() {
    u::header "$1"
    u::header "${BOLD}====${NORMAL}"
}

# print test case descrition
function u::begin_testcase() {
    printf "  $1..."
    echo "" 
    LASTCASE=0
}

# print test case descrition
function u::end_testcase() {
    if [ $LASTCASE != 0 ]; then
        EXIT_CODE=1
    fi
    echo ""
}

# assert values are equal
function u::assert_equal() {
    local expected="${1//[$'\t\r\n']}"
    local actual="${2//[$'\t\r\n']}"
    if [ "$expected" != "$actual" ]; then
        LASTCASE=1
        echo ""
        echo *"$expected"*
        echo "not equal to"
        echo *"$actual"*
    fi
}

# finalize all testsuites
function u::report_and_exit() {
    exit $EXIT_CODE
}