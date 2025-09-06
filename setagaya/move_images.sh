#!/bin/bash

new_registry=asia-northeast1-docker.pkg.dev/setagaya-214807/setagaya
old_registry=gcr.io/setagaya-214807

component=$1
old_image=$old_registry/$component
new_image=$new_registry/$component
docker pull "$old_image"
docker tag "$old_image" "$new_image"
docker push "$new_image"
