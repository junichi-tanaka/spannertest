CREATE TABLE TestTable (
    Key			STRING(MAX) NOT NULL,
    StringValue		STRING(MAX)
) PRIMARY KEY (Key);
CREATE INDEX TestTableByValue ON TestTable(StringValue);
CREATE INDEX TestTableByValueDesc ON TestTable(StringValue DESC);


CREATE TABLE Singers (
    SingerId		INT64 NOT NULL,
    FirstName		STRING(1024),
    LastName		STRING(1024),
    SingerInfo		BYTES(MAX)
) PRIMARY KEY (SingerId);
CREATE INDEX SingerByName ON Singers(FirstName, LastName);

CREATE TABLE Accounts (
    AccountId		INT64 NOT NULL,
    Nickname		STRING(100),
    Balance		INT64 NOT NULL,
) PRIMARY KEY (AccountId);
CREATE INDEX AccountByNickname ON Accounts(Nickname) STORING (Balance);


CREATE TABLE Types (
    RowID		INT64 NOT NULL,
    String		STRING(MAX),
    StringArray		ARRAY<STRING(MAX)>,
    Bytes		BYTES(MAX),
    BytesArray		ARRAY<BYTES(MAX)>,
    Int64a		INT64,
    Int64Array		ARRAY<INT64>,
    Bool		BOOL,
    BoolArray		ARRAY<BOOL>,
    Float64		FLOAT64,
    Float64Array	ARRAY<FLOAT64>,
    Date		DATE,
    DateArray		ARRAY<DATE>,
    Timestamp		TIMESTAMP,
    TimestampArray	ARRAY<TIMESTAMP>,
) PRIMARY KEY (RowID);
