#!/bin/bash

set -e

threshold=30

total_usage=$(df -h --total | tail -1)
percentage=$(echo ${total_usage} | cut -d' ' -f 5 | sed s/%//)

if [[ ${percentage} -gt ${threshold} ]]; then
    echo "warn: disk usage (${percentage}%) exceeds threshold (${threshold}%)"
    exit 1
fi


echo "disk usage (${percentage}%) is below threshold (${threshold}%)"
exit 0

