# Extract properties from .wskprops
AUTH=$(cat ~/.wskprops | grep 'AUTH' | awk -F= '{print $2}')
APIHOST=$(cat ~/.wskprops | grep 'APIHOST' | awk -F= '{print $2}')

# And create secret
kubectl create secret generic seed-defaults-owprops \
    --from-literal=apihost=$APIHOST \
    --from-literal=auth=$AUTH