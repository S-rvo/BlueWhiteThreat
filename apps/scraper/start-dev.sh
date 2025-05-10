#!/bin/bash

# Étape 1 : Exécuter les tests
echo "Running tests..."
go test ./...
if [ $? -ne 0 ]; then
    echo "Tests failed. Aborting."
    exit 1
fi

# Étape 2 : Compiler l'application si les tests passent
echo "Tests passed. Building the application..."
go build -o scraper ./cmd/main.go
if [ $? -ne 0 ]; then
    echo "Build failed. Aborting."
    exit 1
fi

# Étape 3 : Lancer le programme compilé
echo "Build successful. Starting the program..."
./scraper
