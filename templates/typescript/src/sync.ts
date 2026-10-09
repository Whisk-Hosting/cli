import { DeleteObjectCommand, GetObjectCommand, NoSuchKey, PutObjectCommand, S3Client } from "@aws-sdk/client-s3";
import { createHash } from "node:crypto";
import { mkdir, readdir, readFile, rm, writeFile } from "node:fs/promises";
import { dirname, join, relative, sep } from "node:path";

// Working files for batch jobs (CONTRACT.md section 8). The container's disk is /tmp, which is
// emptied on every restart, so a job that keeps a tree of files between runs keeps it in the
// app's bucket and works on a copy: pullTree before the run, pushTree during and after it.
// Each tree has an index of every file's SHA-256 beside it, so a pull downloads only what
// differs from /tmp and a push uploads only what changed. Keep one run per tree at a time with
// a lease row in the database. Needs `storage: true`.

type TreeIndex = Record<string, string>;

// storageClient is the bucket client, or undefined when the manifest did not ask for storage.
export const storageClient = (): S3Client | undefined => {
  const endpoint = process.env.WHISK_STORAGE_ENDPOINT;
  if (!endpoint) return undefined;
  return new S3Client({
    endpoint,
    region: process.env.WHISK_STORAGE_REGION || "us-east-1",
    forcePathStyle: true,
    credentials: { accessKeyId: process.env.WHISK_STORAGE_ACCESS_KEY ?? "", secretAccessKey: process.env.WHISK_STORAGE_SECRET_KEY ?? "" },
  });
};

const bucket = () => process.env.WHISK_STORAGE_BUCKET ?? "";
const treeKey = (tree: string, rel: string) => `${process.env.WHISK_STORAGE_PREFIX ?? ""}trees/${tree}/${rel}`;
const indexKey = (tree: string) => `${process.env.WHISK_STORAGE_PREFIX ?? ""}trees/${tree}.index.json`;

const sha256 = (body: Uint8Array) => createHash("sha256").update(body).digest("hex");

// hashDir is the index of dir as it stands: every regular file's SHA-256. A missing dir is an
// empty tree.
export const hashDir = async (dir: string): Promise<TreeIndex> => {
  const out: TreeIndex = {};
  let entries;
  try {
    entries = await readdir(dir, { recursive: true, withFileTypes: true });
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code === "ENOENT") return out;
    throw err;
  }
  for (const e of entries) {
    if (!e.isFile()) continue;
    const full = join(e.parentPath, e.name);
    out[relative(dir, full).split(sep).join("/")] = sha256(await readFile(full));
  }
  return out;
};

const readIndex = async (c: S3Client, tree: string): Promise<TreeIndex> => {
  try {
    const res = await c.send(new GetObjectCommand({ Bucket: bucket(), Key: indexKey(tree) }));
    return JSON.parse(await res.Body!.transformToString()) as TreeIndex;
  } catch (err) {
    if (err instanceof NoSuchKey) return {};
    throw err;
  }
};

// pullTree makes dir hold exactly the tree's files as last pushed: files that differ are
// downloaded, files the tree no longer has are removed. A tree never pushed leaves dir empty.
export const pullTree = async (c: S3Client, tree: string, dir: string): Promise<void> => {
  const [remote, local] = await Promise.all([readIndex(c, tree), hashDir(dir)]);
  for (const rel of Object.keys(remote).sort()) {
    if (local[rel] === remote[rel]) continue;
    const res = await c.send(new GetObjectCommand({ Bucket: bucket(), Key: treeKey(tree, rel) }));
    const target = join(dir, ...rel.split("/"));
    await mkdir(dirname(target), { recursive: true, mode: 0o700 });
    await writeFile(target, await res.Body!.transformToByteArray(), { mode: 0o600 });
  }
  for (const rel of Object.keys(local)) {
    if (!(rel in remote)) await rm(join(dir, ...rel.split("/")), { force: true });
  }
};

// pushTree makes the tree in the bucket match dir: changed files are uploaded, then the index,
// then files dir no longer has are deleted. A push cut short leaves the previous index in
// place, so the next pull still reads a whole tree and the next push repeats the work.
export const pushTree = async (c: S3Client, tree: string, dir: string): Promise<void> => {
  const [remote, local] = await Promise.all([readIndex(c, tree), hashDir(dir)]);
  for (const rel of Object.keys(local).sort()) {
    if (remote[rel] === local[rel]) continue;
    const body = await readFile(join(dir, ...rel.split("/")));
    await c.send(new PutObjectCommand({ Bucket: bucket(), Key: treeKey(tree, rel), Body: body }));
  }
  await c.send(new PutObjectCommand({ Bucket: bucket(), Key: indexKey(tree), Body: JSON.stringify(local), ContentType: "application/json" }));
  for (const rel of Object.keys(remote)) {
    if (!(rel in local)) await c.send(new DeleteObjectCommand({ Bucket: bucket(), Key: treeKey(tree, rel) }));
  }
};
