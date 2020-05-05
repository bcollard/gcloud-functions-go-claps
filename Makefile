ID_TOKEN=$(shell gcloud auth print-identity-token)
FUNCTION=Clapsgo
GCP_REGION=europe-west1
export PROJECT_ID=personal-218506
export FIRESTORE_ENV=local
export SUPER_USER_MAIL_ADDRESS=baptiste.collard@gmail.com
export REDIS_HOST=localhost
export REDIS_PORT=6379
export FIRESTORE_EMULATOR_HOST=localhost:8081

.ONESHELL:
.PHONY: deploy call-local-get-claps dev call-get-claps notes local-firestore
.DEFAULT_GOAL := help

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'


# LOCAL
dev: ## run the Google Cloud Function NodeJS Framework locally with the parameterized function
	cd cmd && go build && ./cmd

call-local-get-claps: ## call the function locally
	@curl -X GET http://localhost:8080/ -H "Referer: http://localhost:1313/posts/openldap-helm-chart/" -H "Origin: http://localhost:1313"
	#curl -X GET http://localhost:8080/ -H "Referer: http://localhosdt:1313/posts/openldap-helm-chart/"
	#curl -X GET http://localhost:8080/ -H "Referer: https://www.baptistout.net/posts/o?penldap-helm-chart/"



# REMOTE
deploy: ## deploy the function to GCP
	@gcloud functions deploy ${FUNCTION} \
		--entry-point ${FUNCTION} \
		--region ${GCP_REGION} \
		--runtime go113 \
		--trigger-http \
		--timeout 10 \
		--memory 256MB \
		--allow-unauthenticated \
		--set-env-vars PROJECT_ID=${PROJECT_ID} \
		--set-env-vars REDIS_HOST=10.29.74.131 \
		--set-env-vars REDIS_PORT=6379 \
		--vpc-connector projects/${PROJECT_ID}/locations/europe-west1/connectors/bco-serverless-connector \
		--set-env-vars OAUTH_REDIRECT_URI=https://${GCP_REGION}-${PROJECT_ID}.cloudfunctions.net/${FUNCTION}/secure/oauthcallback \
		--set-env-vars SUPER_USER_MAIL_ADDRESS=${SUPER_USER_MAIL_ADDRESS}

call-get-claps: ## call the function deployed on GCP
	@curl -X GET https://${GCP_REGION}-${PROJECT_ID}.cloudfunctions.net/${FUNCTION} -H "Referer: https://www.baptistout.net/posts/openldap-helm-chart/" -H "Origin: https://www.baptistout.net"

