#!/bin/bash

x=0
while true
do
    read -p "Number of servers to run (3-9) [3]: " x
    if [[ "$x" -gt 2 && "$x" -lt 10 ]]; then
      break
    else
      echo "Please input a number between (3-9) [3]: "
    fi
done

sudo docker build -t muteximage .
sudo docker network create --subnet=172.20.0.0/16 mutexnetwork

sudo docker run -d --net mutexnetwork -e APP=frontend --ip 172.20.0.10 -p 8080:8080 muteximage
for (( i = 0; i < x; i++))
do
  sleep 2
  sudo docker run -d --net mutexnetwork -e CLUSTER_ADDRESS=172.20.0.10 muteximage
done
echo DONE!
