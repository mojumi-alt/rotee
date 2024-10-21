#!/bin/bash

trap "trap - SIGTERM && kill -- -$$" SIGINT SIGTERM EXIT

{
    while :
    do
        echo $(date) - Something is happening...
        sleep 1
    done
    # Set output and trigger file, truncate on startup, compress archives, keep max 5 archives, check trigger file every .5 seconds
} | ./rotee -o test.log -t test.trigger -x -c -n 5 -f 0.5 &

{
    while :
    do

        # Trigger rotate if file size reaches 500 bytes.

        filesize=$(stat -c%s test.log)
        if (( filesize > 500 )); then
            echo "Rotate!"
            echo 1 &> test.trigger

            # Lazy, if you really need to know when / if the rotate was successful
            # read the content of the trigger file. 0 means success, 2 means error
            sleep 1
            echo Exit code of rotate: $(cat test.trigger)
        fi
    done
}
