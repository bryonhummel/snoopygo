#!/bin/bash

SCRIPT=$(realpath $0)
SCRIPTDIR=$(dirname $SCRIPT)
echo $SCRIPTDIR
cd $SCRIPTDIR
source devtoken.sh
source livetoken.sh

TOKEN=$DEV_TOKEN

while :
do
    echo "Starting snoopygo"
    ./snoopygo.exe -t $TOKEN
    echo "snoopygo exited with $?"
    sleep 5
done