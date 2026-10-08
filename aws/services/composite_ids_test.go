package awsservices

import (
	awsconv "github.com/theazz/awless-ro/aws/conv"
)

// A few resources have no id of their own in AWS, so awless-ro derives one from the
// fields that identify them: a DNS record by its zone, name, type and set identifier,
// a CloudWatch metric by its namespace and name.
//
// The fixtures work these out rather than spelling the digest, which keeps them
// stating what actually matters — that a record is identified by its zone, name, type
// and set identifier — and
// means changing the digest does not mean editing a table of hex. The hardcoded values
// they replaced had to be updated by hand when the hashing was fixed, and told a reader
// nothing about why those were the identifiers.

func recordID(zoneID, name, recordType, setID string) string {
	return awsconv.HashFields(zoneID, name, recordType, setID)
}

func metricID(namespace, name string) string {
	return awsconv.HashFields(namespace, name)
}
