#!/bin/python3

import argparse
import functools
import io
import subprocess
import sys
import re

def make_pattern(sequence):
    pattern = ''
    for char in sequence:
        if char == ' ':
            char = ''
        pattern += r'\[%s\](?:\[\])?' % char
    return pattern

def main(*in_args):
    parser = argparse.ArgumentParser('Trigger event when specific sequence is read.')
    parser.add_argument(
        'sequence',
        help='Sequence to be accepted',
    )
    parser.add_argument(
        'command_arg', nargs='+',
        help='Arguments of command to be executed when sequence is hit.',
    )
    parser.add_argument(
        '--keep-reading', action='store_true',
        help='Keep reading until EOF is hit no matter if the command was triggered.',
    )
    parser.add_argument(
        '-c', '--count', type=int, default=-1,
        help='How many times should the command be triggered. When set to -1, repead indefinetly.',
    )
    args = parser.parse_args(in_args)
    pattern = make_pattern(args.sequence)
    regexp = re.compile(pattern)
    count = 0
    max_count = args.count
    if max_count < 0:
        max_count = float('inf')
    command = functools.partial(subprocess.Popen, args.command_arg)
    data = io.StringIO()
    while True:
        if count >= max_count:
            if args.keep_reading:
                continue
            else:
                break
        line = sys.stdin.readline()
        if line == '':
            break
        data.write(line.rstrip('\n'))
        #print('Data:', data.getvalue())
        if regexp.search(data.getvalue()):
            data = io.StringIO()
            count += 1
            command()

if __name__ == "__main__":
    sys.exit(main(*sys.argv[1:]))
