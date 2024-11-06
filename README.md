# What is this?

This is a drop-in replacement for tee with built-in logrotate. (rotating tee = rotee)

# Why would i need this?

You want to quickly set up logging for you application to stdout and to a 
file but also want a really simple and highly customizable way of preventing your logfiles from becoming too large or too many, either that or...

# I came here because im trying to run logrotate on EFS and  it is leaving me with null bytes and huge logfiles!

This can happen if a process writes logfiles to a network share and you rotate the logfile
from a different process on another machine.
This tool can help you avoid this problem because every log writer takes care of their own logrotate and rotee actually reopens the file after rotate instead of appening to a potentially stale logfile.

# Installation
We provide pre-built binaries for some platforms, head over to the [release page](https://github.com/mojumi-alt/rotee/releases) to grab the latest version.

You can also build and install yourself (you will need a [go toolchain](https://go.dev/doc/install)):

    git clone https://github.com/mojumi-alt/rotee.git
    cd rotee
    go generate
    go build
    go install

# Okay, how does it work?

By default rotee reads from stdin, so you can try the examples from this section without having to pipe any output into it (just run the rotee commands without the part before `|` ).

The tee part is simple, if your script looks like this now:

    ./my_server.sh | tee -a server.log

Replace tee with rotee:

    ./my_server.sh | rotee -o server.log # Add -x to truncate on startup

Next is to enable logrotate:

    ./my_server.sh | rotee -o server.log -m 5kb # Automatically rotate logfile every 5kb

There is also built-in support for rotating the logfile after a certain time:

    ./my_server.sh | rotee -o server.log -a $((3600*24)) # Automatically rotate logfile every 24 hours

You can also hook your own custom triggers into rotee:

    ./my_server.sh | rotee -o server.log -t ./server.log.trigger

This will make rotee listen to the content of `./server.log.trigger`, writing a single `1` to this file will make rotee rotate the log:

    echo 1 > ./server.log.trigger

At its core that is all there is, you can now of course use all sorts of mechanisms to determine when to write a `1` to the trigger file. You can for example use a cron job to do it daily, do it manually, have an application event trigger it, ...

You can determine whether rotate was successful by reading the content of the file after:

    cat ./server.log.trigger # 0 if okay, 2 if error

# Advanced use

This section explains all configuration options and how to use them.

All of the describe options work together, this means you can use built-in trigger mechanisms with a trigger file, limit file retention and use custom scripts at the same time.

## Append to logfile
Unlike tee this is actually the default mode, see below for explicit truncate.

## Rotate logfile after certain time has passed

    rotee -o output.log -a 86400 # Rotate every 24 hours (expressed in seconds)

This can be done with a built in function, this comes with a few important caveats:

* The time is relative to when rotee started
* If your workload is restarted the timer also restarts
* If you need reliable time based rotation it is recommended to use an external time keeping service (for example cron) in combination with a [trigger file](#using-a-trigger-file).

## Rotate logfile after it reached a certain size (limiting logfile size)
All of the below are equivalent:

    rotee -o output.log -m 5000 # Rotate once the logfile is 5kb
    rotee -o output.log -m 5kb  
    rotee -o output.log -m 0.005mb
    rotee -o output.log -m 0.000005gb

The file size is specified in bytes. The [check frequency](#increase--decrease-trigger-file-polling-frequency) is used to determine how often the file size is checked. If your logfile can grow very quickly (=hundreds of MB per second) it is recommended to adjust this parameter.

## Using a trigger file
Setting up a trigger file for an external service to control rotate can be done like so:

    rotee -o output.log -t test.trigger

Writing a `1` to this file will cause logrotate to happen. After rotate is done you can check the status by reading this file again. `0` indicates success, `2` indicates failure.

The trigger file is checked on startup and then every time the [duration described here passes.](#increase--decrease-trigger-file-polling-frequency)

## Limit number of retained logfiles
This can be used together with the max file age parameter.

    rotee -o output.log -n 5 # Keep 5 most recent logfiles

## Limit max logfile age 
This can be used together with max files parameter. The file modification time (mtime) is used to determine the age of the file.

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
This works with the built-in rotation triggers and with explicit rotation trigger file.

## Turn on additional logging
You can tell rotee to log activities into a separate file using -v parameter.
This will usually not slow down the program at all, so it is save to use in production.

    rotee -o output.log -v activity.log

## Getting started
The best way to get started is to take a look at some examples:

* [Running logrotate on a timer](examples/after_time.sh) 
* [Running logrotate on a certain logfile size](examples/filesize.sh)
* [Using custom compression](examples/custom_compression.sh)

# Questions or problems?
If you are missing any features or have any problems or questions, head over to the [issue tracker](https://github.com/mojumi-alt/rotee/issues) and just open an issue.

# Contributing
Pull requests are welcome!