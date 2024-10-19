#!/bin/bash

{
    while :
    do
        echo $(date) - Something is happening...
        sleep 1
    done
} 2&>1 | rotee 

