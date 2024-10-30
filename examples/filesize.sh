#!/bin/bash

{
    while :
    do
        echo $(date) - Something is happening...
        sleep 1
    done
    # Set output and trigger file, truncate on startup, compress archives, keep max 5 archives, check trigger file every .5 seconds, max file size 500 bytes
} | ./rotee -o test.log -t test.trigger -x -c -n 5 -f 0.5 -m 500
