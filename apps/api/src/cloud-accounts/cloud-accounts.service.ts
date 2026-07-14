import { createHash, randomBytes, randomUUID } from "node:crypto";

import { HttpStatus, Injectable, type OnModuleInit } from "@nestjs/common";
import { InjectModel } from "@nestjs/mongoose";
import {
  BillingEntryType,
  InstanceStatus,
  NotificationType,
  VolumeStatus,
  type ApiKeyView,
  type BillingEntryView,
  type CloudAccountView,
  type NetworkRuleView,
  type NotificationView,
  type OrderView,
  type SshKeyView,
  type VolumeView,
} from "@gpu-rental/contracts";
import { Types, type Model } from "mongoose";

import { DomainException } from "../common/domain-exception";
import { Instance } from "../instances/instance.schema";
import {
  type AttachVolumeDto,
  type CreateApiKeyDto,
  type CreateNetworkRuleDto,
  type CreateSnapshotDto,
  type CreateSshKeyDto,
  type CreateVolumeDto,
  type TopUpDto,
} from "./cloud-accounts.dto";
import {
  type ApiKeyRecord,
  type BillingEntryRecord,
  CloudAccount,
  type CloudAccountDocument,
  type NetworkRuleRecord,
  type NotificationRecord,
  type SshKeyRecord,
  type SnapshotRecord,
  type VolumeRecord,
} from "./cloud-account.schema";

const OPENING_BALANCE_CENTS = 100_000;

@Injectable()
export class CloudAccountsService implements OnModuleInit {
  constructor(
    @InjectModel(CloudAccount.name)
    private readonly accounts: Model<CloudAccount>,
    @InjectModel(Instance.name) private readonly instances: Model<Instance>,
  ) {}

  async onModuleInit(): Promise<void> {
    await this.accounts.init();
  }

  async getAccount(userId: string): Promise<CloudAccountView> {
    return this.toView(await this.ensureAccount(userId));
  }

  async topUp(userId: string, input: TopUpDto): Promise<CloudAccountView> {
    const account = await this.ensureAccount(userId);
    const now = new Date();
    account.balanceCents += input.amountCents;
    account.billingEntries.push(
      this.billingEntry(
        BillingEntryType.TopUp,
        input.amountCents,
        `top-up:${randomUUID()}`,
        "Simulated wallet top-up",
        now,
      ),
    );
    account.notifications.push(
      this.notification(
        NotificationType.Billing,
        "Wallet topped up",
        `A simulated top-up of ${input.amountCents} cents was credited.`,
        now,
      ),
    );
    await account.save();
    return this.toView(account);
  }

  async createSshKey(
    userId: string,
    input: CreateSshKeyDto,
  ): Promise<SshKeyView> {
    const account = await this.ensureAccount(userId);
    const now = new Date();
    const publicKey = input.publicKey.trim();
    const key: SshKeyRecord = {
      id: randomUUID(),
      name: input.name.trim(),
      publicKey,
      fingerprint: `SHA256:${createHash("sha256")
        .update(publicKey)
        .digest("base64")
        .replace(/=+$/, "")}`,
      createdAt: now,
    };
    account.sshKeys.push(key);
    account.notifications.push(
      this.notification(
        NotificationType.Security,
        "SSH key registered",
        `${key.name} can now be selected by simulated instances.`,
        now,
      ),
    );
    await account.save();
    return this.toSshKeyView(key);
  }

  async deleteSshKey(userId: string, keyId: string): Promise<void> {
    const account = await this.ensureAccount(userId);
    const index = account.sshKeys.findIndex((key) => key.id === keyId);
    if (index < 0) this.throwNotFound("SSH_KEY_NOT_FOUND", "SSH key");
    account.sshKeys.splice(index, 1);
    await account.save();
  }

  async createApiKey(
    userId: string,
    input: CreateApiKeyDto,
  ): Promise<ApiKeyView> {
    const account = await this.ensureAccount(userId);
    const now = new Date();
    const token = `gpr_${randomBytes(24).toString("base64url")}`;
    const key: ApiKeyRecord = {
      id: randomUUID(),
      name: input.name.trim(),
      prefix: token.slice(0, 12),
      tokenHash: createHash("sha256").update(token).digest("hex"),
      createdAt: now,
      lastUsedAt: null,
    };
    account.apiKeys.push(key);
    account.notifications.push(
      this.notification(
        NotificationType.Security,
        "API key created",
        `${key.name} was created. Its token is shown once.`,
        now,
      ),
    );
    await account.save();
    return { ...this.toApiKeyView(key), token };
  }

  async deleteApiKey(userId: string, keyId: string): Promise<void> {
    const account = await this.ensureAccount(userId);
    const index = account.apiKeys.findIndex((key) => key.id === keyId);
    if (index < 0) this.throwNotFound("API_KEY_NOT_FOUND", "API key");
    account.apiKeys.splice(index, 1);
    await account.save();
  }

  async createNetworkRule(
    userId: string,
    input: CreateNetworkRuleDto,
  ): Promise<NetworkRuleView> {
    await this.assertMutableInstance(input.instanceId, userId);
    const account = await this.ensureAccount(userId);
    if (
      account.networkRules.some(
        (rule) =>
          rule.instanceId.toString() === input.instanceId &&
          rule.protocol === input.protocol &&
          rule.port === input.port,
      )
    ) {
      throw new DomainException(
        "NETWORK_RULE_EXISTS",
        "The instance already has a rule for this protocol and port",
        HttpStatus.CONFLICT,
      );
    }
    const now = new Date();
    const rule: NetworkRuleRecord = {
      id: randomUUID(),
      instanceId: new Types.ObjectId(input.instanceId),
      name: input.name.trim(),
      protocol: input.protocol,
      port: input.port,
      sourceCidr: input.sourceCidr,
      createdAt: now,
    };
    account.networkRules.push(rule);
    account.notifications.push(
      this.notification(
        NotificationType.Security,
        "Port rule added",
        `${rule.protocol.toUpperCase()} ${rule.port} was added to the simulated firewall.`,
        now,
      ),
    );
    await account.save();
    return this.toNetworkRuleView(rule);
  }

  async deleteNetworkRule(userId: string, ruleId: string): Promise<void> {
    const account = await this.ensureAccount(userId);
    const index = account.networkRules.findIndex((rule) => rule.id === ruleId);
    if (index < 0) {
      this.throwNotFound("NETWORK_RULE_NOT_FOUND", "Network rule");
    }
    account.networkRules.splice(index, 1);
    await account.save();
  }

  async createVolume(
    userId: string,
    input: CreateVolumeDto,
  ): Promise<VolumeView> {
    const account = await this.ensureAccount(userId);
    const now = new Date();
    const volume: VolumeRecord = {
      id: randomUUID(),
      name: input.name.trim(),
      sizeGb: input.sizeGb,
      status: VolumeStatus.Available,
      attachedInstanceId: null,
      monthlyPriceCents: input.sizeGb * 25,
      snapshots: [],
      createdAt: now,
      updatedAt: now,
    };
    account.volumes.push(volume);
    account.notifications.push(
      this.notification(
        NotificationType.Storage,
        "Persistent volume created",
        `${volume.name} (${volume.sizeGb} GB) is ready to attach.`,
        now,
      ),
    );
    await account.save();
    return this.toVolumeView(volume);
  }

  async attachVolume(
    userId: string,
    volumeId: string,
    input: AttachVolumeDto,
  ): Promise<VolumeView> {
    await this.assertMutableInstance(input.instanceId, userId);
    const account = await this.ensureAccount(userId);
    const volume = this.findVolume(account, volumeId);
    if (volume.status === VolumeStatus.Deleted) {
      throw new DomainException(
        "VOLUME_DELETED",
        "A deleted volume cannot be attached",
        HttpStatus.CONFLICT,
      );
    }
    if (
      volume.status === VolumeStatus.Attached &&
      volume.attachedInstanceId?.toString() !== input.instanceId
    ) {
      throw new DomainException(
        "VOLUME_ALREADY_ATTACHED",
        "The volume is attached to another instance",
        HttpStatus.CONFLICT,
      );
    }
    volume.status = VolumeStatus.Attached;
    volume.attachedInstanceId = new Types.ObjectId(input.instanceId);
    volume.updatedAt = new Date();
    await account.save();
    return this.toVolumeView(volume);
  }

  async detachVolume(userId: string, volumeId: string): Promise<VolumeView> {
    const account = await this.ensureAccount(userId);
    const volume = this.findVolume(account, volumeId);
    if (volume.status === VolumeStatus.Deleted) {
      throw new DomainException(
        "VOLUME_DELETED",
        "A deleted volume cannot be detached",
        HttpStatus.CONFLICT,
      );
    }
    volume.status = VolumeStatus.Available;
    volume.attachedInstanceId = null;
    volume.updatedAt = new Date();
    await account.save();
    return this.toVolumeView(volume);
  }

  async createSnapshot(
    userId: string,
    volumeId: string,
    input: CreateSnapshotDto,
  ): Promise<VolumeView> {
    const account = await this.ensureAccount(userId);
    const volume = this.findVolume(account, volumeId);
    if (volume.status === VolumeStatus.Deleted) {
      throw new DomainException(
        "VOLUME_DELETED",
        "A deleted volume cannot be snapshotted",
        HttpStatus.CONFLICT,
      );
    }
    const now = new Date();
    const snapshot: SnapshotRecord = {
      id: randomUUID(),
      name: input.name.trim(),
      sizeGb: volume.sizeGb,
      createdAt: now,
    };
    volume.snapshots.push(snapshot);
    volume.updatedAt = now;
    account.notifications.push(
      this.notification(
        NotificationType.Storage,
        "Snapshot completed",
        `${snapshot.name} was captured from ${volume.name}.`,
        now,
      ),
    );
    await account.save();
    return this.toVolumeView(volume);
  }

  async deleteVolume(userId: string, volumeId: string): Promise<VolumeView> {
    const account = await this.ensureAccount(userId);
    const volume = this.findVolume(account, volumeId);
    if (volume.status === VolumeStatus.Attached) {
      throw new DomainException(
        "VOLUME_ATTACHED",
        "Detach the volume before deleting it",
        HttpStatus.CONFLICT,
      );
    }
    volume.status = VolumeStatus.Deleted;
    volume.attachedInstanceId = null;
    volume.updatedAt = new Date();
    await account.save();
    return this.toVolumeView(volume);
  }

  async markNotificationRead(
    userId: string,
    notificationId: string,
  ): Promise<NotificationView> {
    const account = await this.ensureAccount(userId);
    const notification = account.notifications.find(
      (candidate) => candidate.id === notificationId,
    );
    if (!notification) {
      this.throwNotFound("NOTIFICATION_NOT_FOUND", "Notification");
    }
    notification.readAt ??= new Date();
    await account.save();
    return this.toNotificationView(notification);
  }

  async markAllNotificationsRead(userId: string): Promise<void> {
    const account = await this.ensureAccount(userId);
    const now = new Date();
    for (const notification of account.notifications) {
      notification.readAt ??= now;
    }
    await account.save();
  }

  async chargeOrder(order: OrderView): Promise<void> {
    await this.ensureAccount(order.userId);
    const reference = `order:${order.id}:charge`;
    const now = new Date();
    const account = await this.accounts
      .findOneAndUpdate(
        {
          userId: new Types.ObjectId(order.userId),
          balanceCents: { $gte: order.totalPriceCents },
          "billingEntries.reference": { $ne: reference },
        },
        {
          $inc: { balanceCents: -order.totalPriceCents },
          $push: {
            billingEntries: this.billingEntry(
              BillingEntryType.OrderCharge,
              order.totalPriceCents,
              reference,
              `Booked ${order.instanceName}`,
              now,
            ),
            notifications: this.notification(
              NotificationType.Billing,
              "Order charged",
              `${order.totalPriceCents} cents were reserved for ${order.instanceName}.`,
              now,
            ),
          },
        },
        { new: true },
      )
      .exec();
    if (account) return;
    const current = await this.ensureAccount(order.userId);
    if (current.billingEntries.some((entry) => entry.reference === reference)) {
      return;
    }
    throw new DomainException(
      "INSUFFICIENT_BALANCE",
      "The wallet balance is insufficient for this reservation",
      HttpStatus.PAYMENT_REQUIRED,
    );
  }

  async refundUnused(
    userId: string,
    orderId: string,
    maximumCostCents: number,
    accruedCostCents: number,
  ): Promise<void> {
    const amountCents = Math.max(0, maximumCostCents - accruedCostCents);
    if (amountCents === 0) return;
    await this.ensureAccount(userId);
    const reference = `order:${orderId}:refund`;
    const now = new Date();
    await this.accounts
      .findOneAndUpdate(
        {
          userId: new Types.ObjectId(userId),
          "billingEntries.reference": { $ne: reference },
        },
        {
          $inc: { balanceCents: amountCents },
          $push: {
            billingEntries: this.billingEntry(
              BillingEntryType.OrderRefund,
              amountCents,
              reference,
              "Unused reservation value refunded",
              now,
            ),
            notifications: this.notification(
              NotificationType.Billing,
              "Order refund completed",
              `${amountCents} unused cents were returned to the wallet.`,
              now,
            ),
          },
        },
      )
      .exec();
  }

  async addNotification(
    userId: string,
    type: NotificationType,
    title: string,
    message: string,
  ): Promise<void> {
    await this.ensureAccount(userId);
    await this.accounts
      .updateOne(
        { userId: new Types.ObjectId(userId) },
        {
          $push: {
            notifications: this.notification(type, title, message, new Date()),
          },
        },
      )
      .exec();
  }

  async releaseInstanceResources(
    userId: string,
    instanceId: string,
  ): Promise<void> {
    const account = await this.ensureAccount(userId);
    const now = new Date();
    let changed = false;
    for (const volume of account.volumes) {
      if (volume.attachedInstanceId?.toString() === instanceId) {
        volume.status = VolumeStatus.Available;
        volume.attachedInstanceId = null;
        volume.updatedAt = now;
        changed = true;
      }
    }
    if (changed) await account.save();
  }

  private async ensureAccount(userId: string): Promise<CloudAccountDocument> {
    const now = new Date();
    return (await this.accounts
      .findOneAndUpdate(
        { userId: new Types.ObjectId(userId) },
        {
          $setOnInsert: {
            userId: new Types.ObjectId(userId),
            balanceCents: OPENING_BALANCE_CENTS,
            billingEntries: [
              this.billingEntry(
                BillingEntryType.OpeningCredit,
                OPENING_BALANCE_CENTS,
                "opening-credit",
                "Simulated account opening credit",
                now,
              ),
            ],
            notifications: [
              this.notification(
                NotificationType.Billing,
                "Cloud account ready",
                "Your simulated wallet and operations workspace are ready.",
                now,
              ),
            ],
          },
        },
        { new: true, upsert: true, setDefaultsOnInsert: true },
      )
      .exec()) as CloudAccountDocument;
  }

  private async assertMutableInstance(
    instanceId: string,
    userId: string,
  ): Promise<void> {
    const instance = await this.instances.exists({
      _id: new Types.ObjectId(instanceId),
      userId: new Types.ObjectId(userId),
      status: {
        $in: [
          InstanceStatus.Provisioning,
          InstanceStatus.Running,
          InstanceStatus.Stopped,
        ],
      },
    });
    if (!instance) {
      this.throwNotFound("INSTANCE_NOT_FOUND", "Mutable instance");
    }
  }

  private findVolume(
    account: CloudAccountDocument,
    volumeId: string,
  ): VolumeRecord {
    const volume = account.volumes.find(
      (candidate) => candidate.id === volumeId,
    );
    if (!volume) this.throwNotFound("VOLUME_NOT_FOUND", "Volume");
    return volume;
  }

  private billingEntry(
    type: BillingEntryType,
    amountCents: number,
    reference: string,
    description: string,
    createdAt: Date,
  ): BillingEntryRecord {
    return {
      id: randomUUID(),
      type,
      amountCents,
      reference,
      description,
      createdAt,
    };
  }

  private notification(
    type: NotificationType,
    title: string,
    message: string,
    createdAt: Date,
  ): NotificationRecord {
    return {
      id: randomUUID(),
      type,
      title,
      message,
      readAt: null,
      createdAt,
    };
  }

  private toView(account: CloudAccountDocument): CloudAccountView {
    return {
      wallet: {
        balanceCents: account.balanceCents,
        currency: "CNY",
        updatedAt: account.updatedAt.toISOString(),
      },
      billingEntries: [...account.billingEntries]
        .sort(
          (left, right) => right.createdAt.getTime() - left.createdAt.getTime(),
        )
        .map((entry) => this.toBillingEntryView(entry)),
      sshKeys: [...account.sshKeys]
        .sort(
          (left, right) => right.createdAt.getTime() - left.createdAt.getTime(),
        )
        .map((key) => this.toSshKeyView(key)),
      apiKeys: [...account.apiKeys]
        .sort(
          (left, right) => right.createdAt.getTime() - left.createdAt.getTime(),
        )
        .map((key) => this.toApiKeyView(key)),
      networkRules: [...account.networkRules]
        .sort(
          (left, right) => right.createdAt.getTime() - left.createdAt.getTime(),
        )
        .map((rule) => this.toNetworkRuleView(rule)),
      volumes: [...account.volumes]
        .sort(
          (left, right) => right.createdAt.getTime() - left.createdAt.getTime(),
        )
        .map((volume) => this.toVolumeView(volume)),
      notifications: [...account.notifications]
        .sort(
          (left, right) => right.createdAt.getTime() - left.createdAt.getTime(),
        )
        .map((notification) => this.toNotificationView(notification)),
    };
  }

  private toBillingEntryView(entry: BillingEntryRecord): BillingEntryView {
    return {
      id: entry.id,
      type: entry.type,
      amountCents: entry.amountCents,
      reference: entry.reference,
      description: entry.description,
      createdAt: entry.createdAt.toISOString(),
    };
  }

  private toSshKeyView(key: SshKeyRecord): SshKeyView {
    return {
      id: key.id,
      name: key.name,
      fingerprint: key.fingerprint,
      publicKey: key.publicKey,
      createdAt: key.createdAt.toISOString(),
    };
  }

  private toApiKeyView(key: ApiKeyRecord): ApiKeyView {
    return {
      id: key.id,
      name: key.name,
      prefix: key.prefix,
      token: null,
      createdAt: key.createdAt.toISOString(),
      lastUsedAt: key.lastUsedAt?.toISOString() ?? null,
    };
  }

  private toNetworkRuleView(rule: NetworkRuleRecord): NetworkRuleView {
    return {
      id: rule.id,
      instanceId: rule.instanceId.toString(),
      name: rule.name,
      protocol: rule.protocol,
      port: rule.port,
      sourceCidr: rule.sourceCidr,
      simulated: true,
      createdAt: rule.createdAt.toISOString(),
    };
  }

  private toVolumeView(volume: VolumeRecord): VolumeView {
    return {
      id: volume.id,
      name: volume.name,
      sizeGb: volume.sizeGb,
      status: volume.status,
      attachedInstanceId: volume.attachedInstanceId?.toString() ?? null,
      monthlyPriceCents: volume.monthlyPriceCents,
      snapshots: volume.snapshots.map((snapshot) => ({
        id: snapshot.id,
        name: snapshot.name,
        sizeGb: snapshot.sizeGb,
        createdAt: snapshot.createdAt.toISOString(),
      })),
      createdAt: volume.createdAt.toISOString(),
      updatedAt: volume.updatedAt.toISOString(),
    };
  }

  private toNotificationView(
    notification: NotificationRecord,
  ): NotificationView {
    return {
      id: notification.id,
      type: notification.type,
      title: notification.title,
      message: notification.message,
      readAt: notification.readAt?.toISOString() ?? null,
      createdAt: notification.createdAt.toISOString(),
    };
  }

  private throwNotFound(code: string, subject: string): never {
    throw new DomainException(
      code,
      `${subject} was not found`,
      HttpStatus.NOT_FOUND,
    );
  }
}
