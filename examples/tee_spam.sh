#!/bin/bash
time python3 ./examples/spam.py 5 1000000 2>&1 | tee tee_output.log &> /dev/null