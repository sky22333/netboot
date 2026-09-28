import { test, expect } from "@playwright/test";
import { spawn, type ChildProcess } from "node:child_process";
import { mkdir, mkdtemp } from "node:fs/promises";
import { resolve } from "node:path";
import { createServer } from "node:net";
let server: ChildProcess;
let base: string;

test.beforeAll(async () => {
  const socket = createServer();
  await new Promise<void>((done) => socket.listen(0, "127.0.0.1", done));
  const port = (socket.address() as { port: number }).port;
  await new Promise<void>((done) => socket.close(() => done()));
  const temp = resolve("../tmp/ui-e2e");
  await mkdir(temp, { recursive: true });
  const data = await mkdtemp(resolve(temp, "run-"));
  server = spawn(
    resolve("../dist", process.platform === "win32" ? "pxe.exe" : "pxe"),
    [
      "--data-dir",
      data,
      "--host",
      "127.0.0.1",
      "--port",
      String(port),
      "--no-browser",
    ],
    { windowsHide: true, stdio: "ignore" },
  );
  base = `http://127.0.0.1:${port}`;
  await expect
    .poll(async () => {
      try {
        return (await fetch(base + "/api/v1/setup/status")).status;
      } catch {
        return 0;
      }
    })
    .toBe(200);
});
test.afterAll(async () => {
  if (server && server.exitCode === null) {
    const closed = new Promise<void>((done) =>
      server.once("exit", () => done()),
    );
    server.kill();
    await closed;
  }
});

test("管理后台完整操作与移动端导航", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.addInitScript(() => {
    const NativeSource = window.EventSource;
    (window as any).__testStreams = [];
    window.EventSource = class extends NativeSource {
      constructor(url: string | URL, init?: EventSourceInit) {
        super(url, init);
        (window as any).__testStreams.push(this);
      }
    };
  });
  await page.goto(base);
  await page.getByLabel("用户名", { exact: true }).fill("admin");
  await page.getByLabel("密码", { exact: true }).fill("password123");
  await page.getByRole("button", { name: "创建账号", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "运行概览", exact: true }),
  ).toBeVisible();
  const visit = async (name: string) => {
    await page.getByRole("link", { name, exact: true }).click();
    await expect(
      page.getByRole("heading", { name, exact: true }),
    ).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
  };
  await visit("服务配置");
  await page.getByLabel("最大并发", { exact: true }).fill("17");
  await page.getByRole("switch", { name: "启用 TFTP", exact: true }).click();
  await page
    .getByLabel("普通 DHCP 客户端", { exact: true })
    .selectOption("ignore");
  await page.getByRole("button", { name: "保存配置" }).click();
  await expect(
    page.getByText("已保存，重启服务后生效。", { exact: true }),
  ).toBeVisible();
  await expect(page.locator("[data-sonner-toaster]")).toHaveCSS(
    "position",
    "fixed",
  );
  const config = await (await page.request.get(base + "/api/v1/config")).json();
  expect(config.data.tftp.max_transfers).toBe(17);
  expect(config.data.tftp.enabled).toBe(false);
  expect(config.data.dhcp.non_pxe_action).toBe("ignore");
  await page.screenshot({ path: "../tmp/ui-config.png", fullPage: true });
  await page.getByLabel("最大并发", { exact: true }).fill("18");
  await page.getByRole("link", { name: "文件管理", exact: true }).click();
  await expect(page.getByRole("alertdialog")).toBeVisible();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "取消" })
    .click();
  await page.getByLabel("最大并发", { exact: true }).fill("17");
  await visit("文件管理");
  await page.getByRole("button", { name: "新建", exact: true }).click();
  await page.getByRole("menuitem", { name: "新建文件", exact: true }).click();
  await page
    .getByRole("dialog")
    .getByLabel("名称", { exact: true })
    .fill("test.ipxe");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "确定", exact: true })
    .click();
  await page.getByLabel("文件内容").fill("#!ipxe\necho test");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "保存", exact: true })
    .click();
  await expect(
    page.getByRole("dialog").getByRole("button", { name: "保存", exact: true }),
  ).toBeDisabled();
  await page.getByLabel("文件内容").fill("#!ipxe\necho changed");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "关闭", exact: true })
    .click();
  await expect(page.getByRole("alertdialog")).toBeVisible();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "取消", exact: true })
    .click();
  await expect(page.getByLabel("文件内容")).toHaveValue("#!ipxe\necho changed");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "关闭", exact: true })
    .click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "确认", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.locator("input[type=file]").setInputFiles({
    name: "test.iso",
    mimeType: "application/octet-stream",
    buffer: Buffer.alloc(1024 * 1024, 1),
  });
  await expect(
    page.getByRole("button", { name: "test.iso", exact: true }),
  ).toBeVisible();

  await page.getByRole("button", { name: "test.iso", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "文件详情" })).toBeVisible();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "关闭", exact: true })
    .click();
  await page.getByRole("button", { name: "test.ipxe", exact: true }).click();
  await expect(page.getByLabel("文件内容")).toHaveValue("#!ipxe\necho test");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "关闭", exact: true })
    .click();
  await page
    .getByRole("button", { name: "test.ipxe 的操作", exact: true })
    .click();
  await page.getByRole("menuitem", { name: "重命名", exact: true }).click();
  await page.getByLabel("新名称", { exact: true }).fill("test.iso");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "确定", exact: true })
    .click();
  await expect(
    page
      .getByRole("dialog")
      .getByText("名称已存在，请换一个名称。", { exact: true }),
  ).toBeVisible();
  await page.getByLabel("新名称", { exact: true }).fill("install.ipxe");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "确定", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "install.ipxe", exact: true }),
  ).toBeVisible();
  await page.getByLabel("搜索当前目录").fill("install");
  await expect(
    page.getByRole("button", { name: "test.iso", exact: true }),
  ).toHaveCount(0);
  await page.getByLabel("搜索当前目录").fill("");
  await page.getByRole("button", { name: "新建", exact: true }).click();
  await page.getByRole("menuitem", { name: "新建目录", exact: true }).click();
  await page
    .getByRole("dialog")
    .getByLabel("名称", { exact: true })
    .fill("images");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "确定", exact: true })
    .click();
  await page
    .getByRole("button", { name: "images 的操作", exact: true })
    .click();
  await page.getByRole("menuitem", { name: "查看详情", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "文件详情" })).toBeVisible();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "关闭", exact: true })
    .click();
  await page.getByRole("button", { name: "images", exact: true }).click();
  await expect(
    page.getByText("目录为空，上传文件或点击“新建”。", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "返回上一级", exact: true }).click();
  await page
    .getByRole("button", { name: "images 的操作", exact: true })
    .click();
  await page.getByRole("menuitem", { name: "删除", exact: true }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "确认", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "images", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("tab", { name: "TFTP目录", exact: true }).click();
  await expect(
    page.getByText("存放客户端网络启动所需的固件。", { exact: true }),
  ).toBeVisible();
  await page.getByRole("tab", { name: "HTTP目录", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "install.ipxe", exact: true }),
  ).toBeVisible();

  let finishUpload!: () => void;
  const heldUpload = new Promise<void>((resolve) => {
    finishUpload = resolve;
  });
  let uploadStarted!: () => void;
  const started = new Promise<void>((resolve) => {
    uploadStarted = resolve;
  });
  await page.route("**/api/v1/files/upload?**", async (route) => {
    expect(new URL(route.request().url()).searchParams.get("root")).toBe(
      "http",
    );
    uploadStarted();
    await heldUpload;
    await route.fulfill({ response: await route.fetch() });
  });
  await page.locator("input[type=file]").setInputFiles({
    name: "background.iso",
    mimeType: "application/octet-stream",
    buffer: Buffer.alloc(1024, 2),
  });
  await started;
  await page.getByRole("tab", { name: "TFTP目录", exact: true }).click();
  await expect(
    page.getByText("上传到 HTTP目录/background.iso", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "上传文件", exact: true }),
  ).toBeDisabled();
  finishUpload();
  await expect(page.getByText("上传完成", { exact: true })).toBeVisible();
  await page.unroute("**/api/v1/files/upload?**");
  await expect(
    page.getByRole("button", { name: "background.iso", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("tab", { name: "HTTP目录", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "background.iso", exact: true }),
  ).toBeVisible();
  await page.screenshot({ path: "../tmp/ui-files.png", fullPage: true });
  await visit("设备管理");
  await page.getByRole("button", { name: "添加设备", exact: true }).click();
  await page.getByLabel("名称", { exact: true }).fill("test-pc");
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "test-pc", exact: true }),
  ).toBeVisible();
  await visit("账号管理");
  await page.getByLabel("用户名", { exact: true }).fill("tester");
  await page.getByLabel("密码", { exact: true }).fill("password456");
  await page.getByRole("button", { name: "添加账号", exact: true }).click();
  await expect(page.getByText("tester", { exact: true })).toBeVisible();
  await visit("固件下载");
  await page.getByRole("button", { name: "启动说明", exact: true }).click();
  await expect(
    page.getByRole("dialog").getByRole("heading", { name: "固件启动说明" }),
  ).toBeVisible();
  await page.getByRole("dialog").getByRole("tab", { name: "可选脚本" }).click();
  await expect(
    page.getByRole("dialog").getByText("autoexec.ipxe", { exact: true }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);

  await expect(page.getByRole("tab", { name: "项目固件" })).toHaveAttribute(
    "data-state",
    "active",
  );
  await expect(page.getByRole("tabpanel").getByRole("row")).toHaveCount(4);
  await expect(page.getByText("ipxe-arm64.efi", { exact: true })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "刷新", exact: true }),
  ).toHaveAttribute("data-variant", "ghost");
  const uploadedFirmware = await page.request.post(
    base + "/api/v1/files/upload?root=tftp&path=ipxe-arm64.efi",
    {
      headers: { "Content-Type": "application/octet-stream" },
      data: Buffer.from("test firmware"),
    },
  );
  expect(uploadedFirmware.ok()).toBe(true);
  await page.getByRole("button", { name: "刷新", exact: true }).click();
  await expect(
    page.getByRole("tabpanel").getByText("已下载", { exact: true }),
  ).toBeVisible();
  await page.screenshot({ path: "../tmp/ui-firmware.png", fullPage: true });
  await page.getByRole("tab", { name: "netboot.xyz", exact: true }).click();
  await expect(page.getByRole("tabpanel").getByRole("row")).toHaveCount(5);
  await expect(
    page.getByText("netboot.xyz.efi", { exact: true }),
  ).toBeVisible();
  await page.route("**/api/v1/firmware/download", async (route) => {
    expect(route.request().postDataJSON()).toEqual({
      source: "netboot",
      files: ["netboot.xyz.kpxe"],
    });
    await route.fulfill({
      json: {
        ok: true,
        data: {
          downloads: [
            {
              file: "netboot.xyz.kpxe",
              ok: false,
              error: "下载源返回 HTTP 503，请稍后重试",
            },
          ],
        },
      },
    });
  });
  await page
    .getByRole("tabpanel")
    .getByRole("button", { name: "下载", exact: true })
    .first()
    .click();
  await expect(
    page.getByText("下载源返回 HTTP 503，请稍后重试", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "下载全部", exact: true }),
  ).toBeEnabled();
  await page.unroute("**/api/v1/firmware/download");
  await visit("运行日志");
  expect(
    await page.evaluate(
      () =>
        (window as any).__testStreams.filter(
          (s: EventSource) => s.readyState !== EventSource.CLOSED,
        ).length,
    ),
  ).toBe(1);
  await visit("系统诊断");
  expect(
    await page.evaluate(
      () =>
        (window as any).__testStreams.filter(
          (s: EventSource) => s.readyState !== EventSource.CLOSED,
        ).length,
    ),
  ).toBe(0);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "切换导航" }).click();
  await page.getByRole("link", { name: "设备管理", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "设备管理", exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({ path: "../tmp/ui-mobile.png", fullPage: true });
  await page.setViewportSize({ width: 320, height: 800 });
  for (const name of [
    "服务配置",
    "文件管理",
    "账号管理",
    "固件下载",
    "运行日志",
    "系统诊断",
    "运行概览",
  ]) {
    await page.getByRole("button", { name: "切换导航" }).click();
    await visit(name);
    if (name === "文件管理") {
      await page.getByRole("button", { name: "test.iso", exact: true }).click();
      await expect(
        page.getByRole("dialog", { name: "文件详情" }),
      ).toBeVisible();
      await page.screenshot({
        path: "../tmp/ui-files-mobile-details.png",
        animations: "disabled",
        fullPage: true,
      });
      await page
        .getByRole("dialog")
        .getByRole("button", { name: "关闭", exact: true })
        .click();
      await expect(page.getByRole("dialog")).toHaveCount(0);
      await page.screenshot({
        path: "../tmp/ui-files-mobile.png",
        animations: "disabled",
        fullPage: true,
      });
    }
    if (name === "固件下载") {
      await expect(
        page.getByText("ipxe-arm64.efi", { exact: true }),
      ).toBeVisible();
      await page.getByRole("button", { name: "启动说明", exact: true }).click();
      await expect(page.getByRole("dialog")).toBeVisible();
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      await page.screenshot({
        path: "../tmp/ui-firmware-help-mobile.png",
        fullPage: true,
      });
      await page
        .getByRole("dialog")
        .getByRole("button", { name: "关闭", exact: true })
        .click();
      await page.screenshot({
        path: "../tmp/ui-firmware-mobile.png",
        fullPage: true,
      });
    }
  }
  await page.setViewportSize({ width: 1440, height: 1000 });
  await visit("账号管理");
  await page
    .getByRole("button", { name: "修改密码", exact: true })
    .first()
    .click();
  await page.getByLabel("新密码", { exact: true }).fill("changed-password123");
  await page.getByRole("button", { name: "保存密码", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "登录", exact: true }),
  ).toBeVisible();
  await page.getByLabel("用户名", { exact: true }).fill("admin");
  await page.getByLabel("密码", { exact: true }).fill("changed-password123");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "账号管理", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "退出登录" }).click();
  await expect(
    page.getByRole("button", { name: "登录", exact: true }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});
