#!/bin/bash

BUILD_CMDS=()
PUSH_CMDS=()

# 如果当前系统是 Linux, 
#   BUILD_CMDS 是 docker build。
#   PUSH_CMDS 是 docker push
# 如果当前系统是 MacOS, 
#   BUILD_CMDS 是 container build -a X86_64
#  PUSH_CMDS 是 container image push

os_name=$(uname -s)
case "${os_name}" in
	Linux)
		BUILD_CMDS=(docker build)
		PUSH_CMDS=(docker push)
		;;
	Darwin)
		BUILD_CMDS=(container build -a X86_64)
		PUSH_CMDS=(container image push)
		;;
	*)
		echo "Unsupported OS: ${os_name}" >&2
		if [[ "${BASH_SOURCE[0]}" != "$0" ]]; then
			return 1
		fi
		exit 1
		;;
esac
