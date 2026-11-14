# syntax=docker/dockerfile:1.7
# Neo4j 5.26.33 Community with APOC 5.26.33 core preinstalled.
# The official image otherwise downloads APOC at startup when NEO4J_PLUGINS is set.
FROM neo4j:5.26.33-community

USER root
ADD --checksum=sha256:14223db9bfab8cf6e25f38ab471e6df3cccbd8607afdb06a579aceb1164a283f \
    https://github.com/neo4j/apoc/releases/download/5.26.33/apoc-5.26.33-core.jar \
    /var/lib/neo4j/plugins/apoc.jar
RUN chown neo4j:neo4j /var/lib/neo4j/plugins/apoc.jar

USER neo4j
ENV NEO4J_dbms_security_procedures_unrestricted=apoc.* \
    NEO4J_dbms_security_procedures_allowlist=apoc.*

