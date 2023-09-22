This repo uses Go to embed text, upsert it to a vectorDB and then query it.

Specifically: 
- The input is Whatsapp chat history
- The embedding model is OpenAI's ada-002
- The vector DB is Pinecone

## Steps to run this locally:
1. Obtain an [OpenAI Api Key](https://platform.openai.com/account/api-keys)
2. Obtain a [Pinecone API Key](https://docs.pinecone.io/docs/authentication#finding-your-pinecone-api-key)
3. Save a Whatsapp chat history at the path `"./en_files/en_chat.txt"``
The expected format is `[09.09.23, 14:35:02] ~ john_doe: Hello world!`
4. Run `go run main.go`
5. Follow the instructions - choose action `embed/upsert/query` and then a language, current options are `he/en`. Adding languages simply means another prefix ot the input file name in the `case` block at `main.go`

## Disclaimers
- No tests here, which is not a recommended practice.
- Currently the message text is not stored in the vectorDB. This means that when querying - you get the nearest vector, but not the associated text, which is the search result. This is marked as a TODO. What needs to be done is change the code so that the upsert includes the original text in the metadata for each entry.
- Neither OpenAI nor Pinecone have an official Go client, so it's all cURL commands. Here's where the `debug-commands.txt` comes in handy.
- I am doing this because I think Go is a great choice for AI applications. Benchmarks can be great to prove this point, but are not part of this repo.


## Contact Me
- If you want to make a contribution or reuse the code: go ahead!
- If you want to contact me about this, I'm mostly active on [Twitter](https://twitter.com/nataliepis)