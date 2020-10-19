SHELL=/bin/bash
PROJECT=`jq -r .PROJECT config.json`
REGION=`jq -r .REGION config.json`

echo:
	echo $(PROJECT) $(REGION)

build-app:
	go install ./webmention.go

run:
	go run ./webmention.go

release:
	rm -rf ./build/*
	mkdir -p ./build
	GOBIN=`pwd`/build CGO_ENABLED=0 GOOS=linux go install -a ./webmention.go
	install -d  ./build/usr/local/webmention-run/
	install ./config.json ./build/usr/local/webmention-run/config.json
	cp Dockerfile ./build
	docker build ./build --tag webmention --tag gcr.io/$(PROJECT)/webmention
	docker push gcr.io/$(PROJECT)/webmention

push:
	gcloud beta run deploy webmention --allow-unauthenticated --region $(REGION) --image gcr.io/$(PROJECT)/webmention --project $(PROJECT) --platform managed

start_datastore_emulator:
	 echo To attach run:
	 echo "  export DATASTORE_EMULATOR_HOST=0.0.0.0:8000"
	 docker run -ti -p 8000:8000 google/cloud-sdk:latest gcloud beta emulators datastore start --no-store-on-disk --project test-project --host-port 0.0.0.0:8000

test:
	go test ./...
