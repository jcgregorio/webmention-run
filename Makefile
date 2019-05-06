SHELL=/bin/bash
include ./config.mk
build-app:
	go install ./webmention.go

run:
	go run ./webmention.go

release:
	CGO_ENABLED=0 GOOS=linux go install -a ./webmention.go
	mkdir -p ./build
	rm -rf ./build/*
	cp $(GOPATH)/bin/webmention ./build
	cp Dockerfile ./build
	docker build ./build --tag webmention --tag gcr.io/$(PROJECT)/webmention
	docker push gcr.io/$(PROJECT)/webmention

push:
	gcloud beta run deploy webmention --allow-unauthenticated --region $(REGION) --image gcr.io/$(PROJECT)/webmention --set-env-vars "$(shell cat config.mk | sed 's#export ##' | grep -v "^PORT=" | tr '\n' ',')"

start_datastore_emulator:
	 echo To attach run:
	 echo "  export DATASTORE_EMULATOR_HOST=0.0.0.0:8000"
	 docker run -ti -p 8000:8000 google/cloud-sdk:latest gcloud beta emulators datastore start --no-store-on-disk --project test-project --host-port 0.0.0.0:8000

test:
	go test ./...
