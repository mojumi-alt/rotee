#!/bin/bash

export PYTHONPATH=~/.local/bin

mprof run -C -T 0.01 -o tee_profile.dat ./examples/tee_spam.sh
echo tee output file has $(cat tee_output.log | wc -l) lines
mprof plot -o tee_profile.png tee_profile.dat
rm tee_output.log
rm tee_profile.dat

mprof run -C -T 0.01 -o rotee_profile.dat ./examples/rotee_spam.sh
gunzip rotee_output.log*.gz
echo rotee output file has $(cat rotee_output.log* | wc -l) lines
mprof plot -o rotee_profile.png rotee_profile.dat
rm rotee_output.log*
rm rotee_profile.dat