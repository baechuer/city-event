#!/bin/bash
set -e

echo "Initialising BFF Service Tests..."
cd services/bff-service

echo "Running Go Tests..."
go test ./... -v

echo "BFF Service Tests Passed!"
