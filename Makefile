
export SPANNER_PROJECT_ID=GCP_TEST_PROJECT_ID
export SPANNER_INSTANCE_ID=spanner_instance_id
export SPANNER_DATABASE_ID=spanner_database_id

export SPANNER_EMULATOR_HOST=localhost:9010

.PHONY: all
all: test

.PHONY: test
test: test/integration

.PHONY: test/integration
test/integration: emulator/start
	go test -v -timeout 10m ./... -run '^TestIntegration_' 2>&1

.PHONY: emulator/start
emulator/start:
	docker-compose up -d

.PHONY: emulator/stop
emulator/stop:
	docker-compose down -t 60
