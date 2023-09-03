help: ## Show this help.
	@fgrep -h "##" $(MAKEFILE_LIST) | fgrep -v fgrep | sed -e 's/\\$$//' | sed -e 's/##//' | sort

sync_production: ## Sync to production
	aws-vault exec production -- aws s3 sync . s3://euw2-p-all-a-api-gateway-skipper-8sferpoi

sync_dev: ## Sync to dev
	aws-vault exec dev -- aws s3 sync . s3://euw2-d-all-a-api-gateway-skipper-y5yqa82l