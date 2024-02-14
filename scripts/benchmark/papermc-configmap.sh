#!/bin/sh

# Set kubeconfig
export KUBECONFIG=$HOME/.kube/fledge2.yml
export IMAGE=papermc

# Deploy container
sudo kubectl delete "configmaps/$IMAGE"
sudo kubectl apply -f - << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: $IMAGE
data:
  banned-players.json: |
    []
  banned-ips.json: |
    []
  ops.json: |
    [
        {
            "uuid": "ec9e6643-8bdb-3fc2-977d-e73f0bb1cb29",
            "name": "MaxDuClark",
            "level": 4
        },
        {
            "uuid": "56b13711-e1be-3ae5-954f-390c9ebab470",
            "name": "Player 0",
            "level": 4
        },
        {
            "uuid": "7d47ab93-4c5b-30ee-abb9-99f58321567e",
            "name": "Player 1",
            "level": 4
        },
        {
            "uuid": "b58e6638-c98b-3fe8-b302-13fb46110906",
            "name": "Player 2",
            "level": 4
        },
        {
            "uuid": "398be230-de2c-3009-aa1a-ecd53565ed8a",
            "name": "Player 3",
            "level": 4
        },
        {
            "uuid": "0119c200-16b7-31a7-ab23-2546619a3b4b",
            "name": "Player 4",
            "level": 4
        },
        {
            "uuid": "8e0fc8ca-5029-3b1d-bc53-ade4b1398101",
            "name": "Player 5",
            "level": 4
        },
        {
            "uuid": "c6c19e55-622a-3da5-b29b-a423b717b559",
            "name": "Player 6",
            "level": 4
        },
        {
            "uuid": "3ac50195-1d2c-36f8-b889-04916f02e85e",
            "name": "Player 7",
            "level": 4
        },
        {
            "uuid": "4d9d45d2-bc0a-3b49-96ba-f068406f222d",
            "name": "Player 8",
            "level": 4
        },
        {
            "uuid": "d658c4dc-298e-38a0-bd85-10bcf8698bb7",
            "name": "Player 9",
            "level": 4
        },
    ]
  whitelist.json: |
    []
  server.properties: |
    #Minecraft server properties
    #(File Modification Datestamp)
    generator-settings=
    op-permission-level=4
    allow-nether=true
    level-name=world
    enable-query=false
    allow-flight=false
    announce-player-achievements=true
    server-port=25565
    max-world-size=29999984
    level-type=DEFAULT
    enable-rcon=false
    level-seed=84579735
    force-gamemode=false
    server-ip=
    network-compression-threshold=256
    max-build-height=256
    spawn-npcs=true
    white-list=false
    spawn-animals=true
    hardcore=false
    snooper-enabled=true
    resource-pack-sha1=
    online-mode=false
    resource-pack=
    pvp=true
    difficulty=1
    enable-command-block=true
    gamemode=1
    player-idle-timeout=0
    max-players=20
    max-tick-time=60000
    spawn-monsters=true
    generate-structures=true
    use-native-transport=false
    view-distance=10
    motd=A Minecraft Server
  bukkit.yml: |
    settings:
        connection-throttle: 1
EOF
