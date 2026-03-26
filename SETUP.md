
# Run vespa

```bash
docker run --detach --name vespa --hostname vespa-container \
  --publish 8080:8080 --publish 19071:19071 \
  vespaengine/vespa
```

- verify it is running
```bash
curl -s http://localhost:19071/status.html
<title>OK</title>
```
- vespa CLI
```bash
brew install vespa-cli
```

- set target 
```bash
# 1. Tell the CLI to talk to your local Docker container
vespa config set target local

$ vespa status
Error: deployment not converged: got status 404 
# means no applications available yet, we need to install it


```

# Deploying the schema
- first you need to create your app package
```bash
slack_app/ # top dir to deploy
├── services.xml           <-- Defines the "Infrastructure"
└── schemas/               <-- Folder for your "Tables"
    └── slack_message.sd   <-- The actual Schema definition
```

- `services.xml` This tells Vespa to start one cluster for storing data and 
   one "container" to handle your Go API requests.
```xml
<?xml version="1.0" encoding="utf-8" ?>
<services version="1.0">
  <container id="default" version="1.0">
    <document-api /> <search /> 
     <nodes><node hostalias="node1" /></nodes>
  </container>

  <content id="slack_content" version="1.0">
    <redundancy>1</redundancy>
    <documents>
      <document type="slack_message" mode="index" />
    </documents>
     <nodes>
        <node hostalias="node1" distribution-key="0" />
     </nodes>
  </content>
</services>
```

# This is your "Table Definition. it defines the Slack data and the AI vector.
- `schemas/slack_message.sd` 
```
schema slack_message {
    document slack_message {
        field channel_id type string {
            indexing: summary | attribute
        }
        field content type string {
            indexing: summary | index
        }
        field embedding type tensor<float>(x[384]) {
            indexing: attribute | index
            attribute { distance-metric: angular }
        }
    }
    
    rank-profile hybrid_search {
        inputs {
            query(user_vector) tensor<float>(x[384])
        }
        first-phase {
            expression: closeness(field, embedding)
        }
    }
}
```



```bash

#When you run vespa deploy, the CLI sends your files to the Config Server (Port 19071). 
#However, just because the files are uploaded doesn't mean the database is ready to handle data.
#Convergence: Vespa has to "converge" the state. It looks at your schema, 
#allocates memory for vectors, and starts the search processes.
#The Timer: --wait 300 tells the CLI: "Don't exit back to the terminal prompt until the system is 100% ready or 300 seconds have passed."
#Why it fails: If you see an error after 300 seconds, it's almost always because the Docker container doesn't have enough RAM to start the services (Vespa needs 4GB-6GB).
#tell Vespa about your Slack schema. Go to your terminal in the directory where your slack_app folder lives and run:
 vespa deploy vespa_schema --wait 300
#Uploading application package... done
#Success: Deployed 'vespa_schema' with session ID 3
#Waiting up to 5m0s for deployment to converge...
docker logs vespa
#[2026-03-11 17:00:51.269] INFO    container        Container.com.yahoo.jrt.slobrok.api.Register	[RPC @ tcp/vespa-container:19101] registering default/container.0/chain.indexing with location broker tcp/vespa-container:19099 completed successfully

$ vespa status
#Container default at http://127.0.0.1:8080 is ready

```

# Schema updates
- In the world of Vespa, there is no separate "migration" command or script like you’d find in SQL (e.g., migrate up or Liquibase).

The schema is the migration. When you change your .sd file and run vespa deploy, Vespa compares your new schema to the one currently running and handles the transition automatically.

- for example i am adding new field to schema and run  ` vespa deploy vespa_schema --wait 300` again to apply the change
- `How "Migrations" Work in Vespa`
Vespa is built for "Live Schema Evolution." This means it can change its internal database structure while you are still sending queries and data.
Adding a Field (Safe): When you add field timestamp type long, Vespa simply starts allowing that field in new documents. Existing documents will just have a "null" or 0 value for that field until you update them.
Changing Rank Profiles (Safe): You can change how you score documents as often as you like; this doesn't change the data, only the search logic.
Destructive Changes (Guarded): If you try to do something "scary"—like changing a field from a string to a long (which would break existing data)—Vespa will actually block the deployment and tell you that you need a "Validation Override."
- The `Validation Override` 
  If you ever need to do a destructive change, the vespa deploy command will fail with an error like:
  change-id-field-type: field 'timestamp' changed type from long to string.
  To force it, you have to add a temporary "permission slip" to a file called validation-overrides.xml in your app folder:
```xml
<alidation-overrides>
   <allow until="2026-03-15">content-type-change</allow>
</validation-overrides>
```