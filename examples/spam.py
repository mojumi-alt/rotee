import multiprocessing
import sys
import logging
import random
import string
import os
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s (%(funcName)s): %(message)s",
    datefmt="%Y-%m-%d,%H:%M:%S",
)

def make_random_data(N):
    return ''.join(random.choices(string.ascii_uppercase + string.digits, k=N))

def run(line_count):
    pid = os.getpid()
    for _ in range(line_count):
        logging.info(str(pid)+ ": "+ make_random_data(100))

if __name__ == '__main__':

    proc_count = int(sys.argv[1])
    lines_per_proc = int(sys.argv[2])

    procs = [multiprocessing.Process(target=run, args=(lines_per_proc,)) for _ in range(proc_count)]
    for p in procs:
        p.start()

    for p in procs:
        p.join()