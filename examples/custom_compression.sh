#!/bin/bash

trap "trap - SIGTERM && kill -- -$$" SIGINT SIGTERM EXIT

{
    while :
    do
        echo $(date) - Something is happening...
        sleep 1
    done
    # Set output and trigger file, truncate on startup, keep max 5 archives, check trigger file every .5 seconds
    # Note the absence of -c flag, so the output file is not compressed by rotee
    # Instead we do it ourselves with a post script
} | ./rotee -o test.log -t test.trigger -x -n 5 -f 0.5 \
    -p "zip \$0.zip \$0 && mv \$0.zip \$0" &

{
    while :
    do
        # Rotate log file every 5 seconds
        # You could also set up a cron job that does this write
        sleep 5
        echo "Rotate!"
        echo 1 &> test.trigger

        # Lazy, if you really need to know when / if the rotate was successful
        # read the content of the trigger file. 0 means success, 2 means error
        sleep 1
        echo Exit code of rotate: $(cat test.trigger)
    done
}
