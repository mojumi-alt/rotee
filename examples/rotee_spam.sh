#!/bin/bash

{
    while :
    do
        sleep 30
        echo 1 &> test.trigger
    done
} &

time python3 ./examples/spam.py 5 1000000 2>&1 | ./rotee -o rotee_output.log -t test.trigger -f 0.1 -x -c &> /dev/null
rm test.trigger
pkill -f ./examples/rotee_spam.sh