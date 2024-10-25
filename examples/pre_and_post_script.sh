#!/bin/bash

trap "trap - SIGTERM && kill -- -$$" SIGINT SIGTERM EXIT

{
    while :
    do
        echo $(date) - Something is happening...
        sleep 1
    done
    # Set output and trigger file, truncate on startup, compress archives, keep max 5 archives, check trigger file every .5 seconds
    # Set pre and post script
    # In this setting we need to make sure that $ is escaped so we dont eval it here but pass it to the actual script
    # You can run any arbitrary shell command, for example "python3 myscript.py \$0" to run a python script on the file 
} | ./rotee -o test.log -t test.trigger -x -c -n 5 -f 0.5 \
    -s "echo $(date) rotate pre script \$0 | tee pre_script_output.log" \
    -p "echo $(date) rotate post script \$0 | tee post_script_output.log" &

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
