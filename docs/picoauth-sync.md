# Pico Auth: sync between computers

Design decisions for keeping the same Pico Auth vault on several computers
(for example a Mac at home and a Windows PC at work). Nothing here is built yet.

## Principle

- Sync is **optional**: without GitHub, Pico Auth works on one computer as it does today.
- The encrypted vault lives in a **private GitHub repository** of the user (for example `picoauth-vault`).
- Pico Auth **does not use git**: it talks to GitHub directly.

## Access to GitHub

- Pico Auth is registered **once**, by its author, as a public **GitHub App**.
  Its Client ID is in the code: it is free and not a secret.
- Each user installs the app **only on their own repository**.
- On each computer the link is made by **logging in to GitHub from the browser**
  (a short code + Authorize): no token to create by hand.

## Security

- The vault on GitHub opens only with the **master password + a second key**.
- The second key lives **only on the linked computers**, never on GitHub.
- A new computer receives it when **an already linked computer approves it**,
  after comparing a **6-digit code** shown on both screens.

## Everyday use

- Each computer works on its **local copy** of the vault.
- When Pico Auth opens, and from time to time while it is open, it reads the repository:
  a newer version of the vault, and requests from new computers waiting for approval.
- Every change is **uploaded at once**.
- If two computers save at almost the same time, GitHub refuses the older upload:
  Pico Auth downloads the newer vault and applies the single change again on it.

## First start on a computer

```
1  New vault
2  Open an existing vault  →  From GitHub / From a backup file
```

The labels are still to be improved.

## Local vault and GitHub

- A local vault can be linked to GitHub **only if the repository has no vault yet**.
  Two vaults are never merged.
- Who has a local vault and wants to use one already on GitHub can either
  **delete the local vault** or use **an empty repository**.
- A local vault can be deleted **only if it is not linked to GitHub**,
  by typing **delete** and its **master password**.

## Still open

- What Pico Auth does without internet when an account is changed.
- How to unlink or remove a computer.
- The key and screen to link an existing local vault to GitHub.
- The labels of the first-start menu.
