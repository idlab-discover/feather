#!/bin/sh
set -xe
trap 'pkill -P $$' EXIT INT
ssh -L 127.0.0.1:25565:worker0.fledge2.ilabt-imec-be.wall1.ilabt.iminds.be:25565 worker0 sleep 1000000
