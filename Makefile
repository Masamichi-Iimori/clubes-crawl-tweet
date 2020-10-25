include .env


$(eval export $(shell sed -ne 's/ *#.*$//; /./ s/=.*$$// p' .env))

.PHONY: build

build:
	sam build

packege:
	sam package --template-file template.yaml --output-template-file output-template.yaml --s3-bucket clubes-crawltweet --profile masamichi

deploy:
	#sam deploy --template-file output-template.yaml --stack-name clubes-crawltweet --capabilities CAPABILITY_IAM --profile masamichi
	sam deploy --stack-name clubes-crawltweet --s3-bucket clubes-crawltweet --capabilities CAPABILITY_IAM --profile masamichi --parameter-overrides AwsAccessKey=${AWS_ACCEESS_KEY} AwsSecretAccessKey=${AWS_SECRET_ACCEESS_KEY} ConsumerKey=${CONSUMER_KEY} ConsumerSecret=${CONSUMER_SECRET} AccessToken=${ACCESS_TOKEN} AccessTokenSecret=${ACCESS_TOKEN_SECRET}

delete:
	aws cloudformation delete-stack --stack-name clubes-crawltweet

local:
	#sam local start-api
	sam build && sam local start-api --parameter-overrides AwsAccessKey=${AWS_ACCEESS_KEY} AwsSecretAccessKey=${AWS_SECRET_ACCEESS_KEY} ConsumerKey=${CONSUMER_KEY} ConsumerSecret=${CONSUMER_SECRET} AccessToken=${ACCESS_TOKEN} AccessTokenSecret=${ACCESS_TOKEN_SECRET}

invoke:
	sam build && sam local invoke --parameter-overrides AwsAccessKey=${AWS_ACCEESS_KEY} AwsSecretAccessKey=${AWS_SECRET_ACCEESS_KEY} ConsumerKey=${CONSUMER_KEY} ConsumerSecret=${CONSUMER_SECRET} AccessToken=${ACCESS_TOKEN} AccessTokenSecret=${ACCESS_TOKEN_SECRET}
