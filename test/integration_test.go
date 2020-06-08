package test_test

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	instance "cloud.google.com/go/spanner/admin/instance/apiv1"
	adminpb "google.golang.org/genproto/googleapis/spanner/admin/database/v1"
	instancepb "google.golang.org/genproto/googleapis/spanner/admin/instance/v1"
	"google.golang.org/grpc/codes"
)

var (
	databaseAdmin *database.DatabaseAdminClient
	instanceAdmin *instance.InstanceAdminClient

	testProjectID  = os.Getenv("SPANNER_PROJECT_ID")
	testInstanceID = os.Getenv("SPANNER_INSTANCE_ID")
	dbName         = os.Getenv("SPANNER_DATABASE_ID")

	schemaDDL = "../db/spanner/schema.sql"

	testTable        = "TestTable"
	testTableIndex   = "TestTableByValue"
	testTableColumns = []string{"Key", "StringValue"}
)

type testTableRow struct{ Key, StringValue string }

func TestMain(m *testing.M) {
	cleanup := initIntegrationTests()
	res := m.Run()
	cleanup()
	os.Exit(res)
}

func initIntegrationTests() (cleanup func()) {
	ctx := context.Background()
	flag.Parse() // Needed for testing.Short().
	noop := func() {}

	if testing.Short() {
		log.Println("Integration tests skipped in -short mode.")
		return noop
	}

	if testProjectID == "" {
		fmt.Println(testProjectID)
		log.Println("Integration tests skipped: GCP_PROJECT_ID is missing")
		return noop
	}

	var err error
	// Create InstanceAdmin and DatabaseAdmin clients.
	instanceAdmin, err = instance.NewInstanceAdminClient(ctx)
	if err != nil {
		log.Fatalf("cannot create instance databaseAdmin client: %v", err)
	}
	databaseAdmin, err = database.NewDatabaseAdminClient(ctx)
	if err != nil {
		log.Fatalf("cannot create databaseAdmin client: %v", err)
	}

	// Get the list of supported instance configs for the project that is used
	// for the integration tests. The supported instance configs can differ per
	// project. The integration tests will use the first instance config that
	// is returned by Cloud Spanner. This will normally be the regional config
	// that is physically the closest to where the request is coming from.
	configIterator := instanceAdmin.ListInstanceConfigs(ctx, &instancepb.ListInstanceConfigsRequest{
		Parent: fmt.Sprintf("projects/%s", testProjectID),
	})
	config, err := configIterator.Next()
	if err != nil {
		log.Fatalf("Cannot get any instance configurations.\nPlease make sure the Cloud Spanner API is enabled for the test project.\nGet error: %v", err)
	}

	// Create a test instance to use for this test run.
	op, err := instanceAdmin.CreateInstance(ctx, &instancepb.CreateInstanceRequest{
		Parent:     fmt.Sprintf("projects/%s", testProjectID),
		InstanceId: testInstanceID,
		Instance: &instancepb.Instance{
			Config:      config.Name,
			DisplayName: testInstanceID,
			NodeCount:   1,
		},
	})
	if err != nil {
		log.Fatalf("could not create instance with id %s: %v", fmt.Sprintf("projects/%s/instances/%s", testProjectID, testInstanceID), err)
	}

	// Wait for the instance creation to finish.
	i, err := op.Wait(ctx)
	if err != nil {
		log.Fatalf("waiting for instance creation to finish failed: %v", err)
	}
	if i.State != instancepb.Instance_READY {
		log.Printf("instance state is not READY, it might be that the test instance will cause problems during tests. Got state %v\n", i.State)
	}

	return func() {
		databaseAdmin.Close()
		instanceAdmin.Close()
	}
}

// Prepare initializes Cloud Spanner testing DB and clients.
func prepareIntegrationTest(ctx context.Context, t *testing.T, schemaFileName string) (*spanner.Client, string, func()) {
	t.Helper()

	if databaseAdmin == nil {
		t.Skip("Integration tests skipped")
	}

	b, err := ioutil.ReadFile(schemaFileName)
	if err != nil {
		t.Fatal(err)
	}
	statements := strings.Split(string(b), ";")
	statements = statements[:len(statements)-1]

	dbPath := fmt.Sprintf("projects/%v/instances/%v/databases/%v", testProjectID, testInstanceID, dbName)
	// Create database and tables.
	op, err := databaseAdmin.CreateDatabaseWithRetry(ctx, &adminpb.CreateDatabaseRequest{
		Parent:          fmt.Sprintf("projects/%v/instances/%v", testProjectID, testInstanceID),
		CreateStatement: "CREATE DATABASE " + dbName,
		ExtraStatements: statements,
	})
	if err != nil {
		t.Fatalf("cannot create testing DB %v: %v", dbPath, err)
	}
	if _, err := op.Wait(ctx); err != nil {
		t.Fatalf("cannot create testing DB %v: %v", dbPath, err)
	}
	client, err := NewSpannerClient(ctx, dbPath)
	if err != nil {
		t.Fatalf("cannot create data client on DB %v: %v", dbPath, err)
	}
	return client, dbPath, func() {
		client.Close()
	}
}

// NewSpannerClient creates Cloud Spanner data client.
func NewSpannerClient(ctx context.Context, dbPath string) (client *spanner.Client, err error) {
	config := spanner.ClientConfig{
		NumChannels: 1,
		SessionPoolConfig: spanner.SessionPoolConfig{
			MaxOpened: 1,
			MinOpened: 1,
		},
	}
	client, err = spanner.NewClientWithConfig(ctx, dbPath, config)
	if err != nil {
		return nil, fmt.Errorf("cannot create data client on DB %v: %v", dbPath, err)
	}
	return client, nil
}

func TestIntegration_Reads(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Set up testing environment.
	client, _, cleanup := prepareIntegrationTest(ctx, t, schemaDDL)
	defer cleanup()

	// Includes k0..k14. Strings sort lexically, eg "k1" < "k10" < "k2".
	var ms []*spanner.Mutation
	for i := 0; i < 15; i++ {
		ms = append(ms, spanner.InsertOrUpdate(testTable,
			testTableColumns,
			[]interface{}{fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i)}))
	}
	// Don't use ApplyAtLeastOnce, so we can test the other code path.
	if _, err := client.Apply(ctx, ms); err != nil {
		t.Fatal(err)
	}

	// Point read.
	row, err := client.Single().ReadRow(ctx, testTable, spanner.Key{"k1"}, testTableColumns)
	if err != nil {
		t.Fatal(err)
	}
	var got testTableRow
	if err := row.ToStruct(&got); err != nil {
		t.Fatal(err)
	}
	if want := (testTableRow{"k1", "v1"}); got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	// Point read not found.
	_, err = client.Single().ReadRow(ctx, testTable, spanner.Key{"k999"}, testTableColumns)
	if spanner.ErrCode(err) != codes.NotFound {
		t.Fatalf("got %v, want NotFound", err)
	}

	// Index point read.
	rowIndex, err := client.Single().ReadRowUsingIndex(ctx, testTable, testTableIndex, spanner.Key{"v1"}, testTableColumns)
	if err != nil {
		t.Fatal(err)
	}
	var gotIndex testTableRow
	if err := rowIndex.ToStruct(&gotIndex); err != nil {
		t.Fatal(err)
	}
	if wantIndex := (testTableRow{"k1", "v1"}); gotIndex != wantIndex {
		t.Errorf("got %v, want %v", gotIndex, wantIndex)
	}
	// Index point read not found.
	_, err = client.Single().ReadRowUsingIndex(ctx, testTable, testTableIndex, spanner.Key{"v999"}, testTableColumns)
	if spanner.ErrCode(err) != codes.NotFound {
		t.Fatalf("got %v, want NotFound", err)
	}

	stmt := spanner.NewStatement("WITH X AS (SELECT 7 AS Z) SELECT * FROM X")
	iter := client.Single().Query(ctx, stmt)
	defer iter.Stop()

	var i int64
	row, err = iter.Next()
	if err != nil {
		t.Errorf("query failed with %v", err)
	}
	if err = row.Columns(&i); err != nil {
		t.Errorf("failed to parse row %v", err)
	}
}
