# What is this?

This is a drop-in replacement for tee with built-in logrotate. (rotating tee = rotee)

# Why would i need this?

You want to quickly set up logging for you application to stdout and to a 
file but also want a really simple way of preventing your logfiles from becoming too large or too many, either that or...

# I came here because im trying to run logrotate on EFS and  it is leaving me with null bytes and huge logfiles!

This can happen if a process writes logfiles to a network share and you rotate the logfile
from a different process on another machine.
This tool can help you avoid this problem because every log writer takes care of their own logrotate and rotee actually reopens the file after rotate instead of appening to a potentially stale logfile.

# Okay, how does it work?

The tee part is simple, if your script looks like this now:

    ./my_server.sh | tee -a server.log

Replace tee with rotee:

    ./my_server.sh | rotee -o server.log # Add -x to truncate on startup

Next is to enable logrotate

    ./my_server.sh | rotee -o server.log -t ./server.log.trigger

This will make rotee listen to the content of `./server.log.trigger`, writing a single `1` to this file will make rotee rotate the log:

    echo 1 > ./server.log.trigger

At its core that is all there is, you can now of course use all sorts of mechanisms to determine when to write a `1` to the trigger file. You can for example use a cron job to do it daily, use a file size monitor, ...

You can determine whether rotate was successful by reading the content of the file after:

    cat ./server.log.trigger # 0 if okay, 2 if error

# Advanced use

This section explains all configuration options and how to use them.

## Append to logfile
Unlike tee this is actually the default mode, see below for explicit truncate.

## Limit number of retained logfiles
This can be used together with the max file age parameter.

    rotee -o output.log -n 5 # Keep 5 most recent logfiles

## Limit max logfile age 
This can be used together with max files parameter.

    rotee -o output.log -d 30 # Delete all logfiles older than 30 days

## Truncate logfile on startup

    rotee -o output.log -x # Default is append to logfile on startup

## Increase / decrease trigger file polling frequency
If your workload needs extremly fast response times for logrotate use this, of course this uses up more IO bandwidth and CPU.

    rotee -o output.log -f 0.01 # Poll file every 0.01 seconds, default is 1 second

On the other hand if your workload can wait with rotating you can decrease the frequency to save IO bandwidth and CPU:

    rotee -o output.log -f 60 # Poll file every 60 seconds, default is 1 second, perfect for daily logrotate

## Compress logfiles
We offer gzip compression out of the box. Note that the default for compression is OFF.

    rotee -o output.log -c

## Running custom scripts on rotate
If you need to customize the behavior we offer pre-rotate and post-rotate scripts:

    rotee -o output.log -s "echo \$0" -p "echo \$0"

The pre-script (-s) is executed on the file before its rotated, the post-script (-p) is executed on the file after rotate is done. 

## Getting started
The best way to get started is to take a look at some examples:

* [Running logrotate on a timer](examples/after_time.sh) 
* [Running logrotate on a certain logfile size](examples/filesize.sh)
* [Using custom compression](examples/custom_compression.sh)
