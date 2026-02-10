#!/bin/bash
set -xe
python main.py --dataset 'evanellis/Codeforces-Python-Submissions_correct' \
    --split 'test' --output ../../datasets/codeforces.test.jsonl

python main.py --dataset 'evanellis/Codeforces-Python-Submissions_correct' \
    --split 'train' --output ../../datasets/codeforces.train.jsonl
