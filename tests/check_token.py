import json

with open(r'd:\bse\data\tokens\token_details.json') as f:
    tokens = json.load(f)

token_id = '861201'
if token_id in tokens:
    print(f'Token {token_id} found:')
    print(json.dumps(tokens[token_id], indent=2))
else:
    print(f'Token {token_id} NOT FOUND')

# Check the values at offset 4-7 (38005 paise = 380.05 Rupees)
print(f'\n38005 paise = {38005/100:.2f} Rupees')
print(f'38235 paise = {38235/100:.2f} Rupees') 
print(f'47585 paise = {47585/100:.2f} Rupees')

# These look like reasonable option prices!
