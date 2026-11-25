/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

// 渠道定时下线的时间换算。
//
// 数据库里存的是 Unix 秒（绝对时刻），而界面上填的是墙上时间（年月日时分）。
// 两者之间必须绑定一个时区：这里统一使用服务端下发的时间口径，而不是浏览器本地时区，
// 否则同一个「12-01 00:00」在不同时区的管理员手里会命中不同时刻（相差 8 小时）。

// 默认口径与后端一致：UTC+8。
export const DEFAULT_CHANNEL_SCHEDULE_OFFSET_MINUTES = 480;

const pad2 = (value) => String(value).padStart(2, '0');

// 从 /api/status 数据中读取时区偏移（分钟）。缺失或非法时回退到 UTC+8，
// 保证老后端 / 缓存数据下行为与默认配置一致。
export function resolveChannelScheduleOffsetMinutes(status) {
  const offset = Number(status?.channel_schedule_utc_offset_minutes);
  if (!Number.isFinite(offset) || Math.abs(offset) > 14 * 60) {
    return DEFAULT_CHANNEL_SCHEDULE_OFFSET_MINUTES;
  }
  return offset;
}

// 把分钟偏移格式化成提示文案里的时区标识，例如 480 -> UTC+8。
export function formatChannelScheduleOffsetLabel(offsetMinutes) {
  const offset = Number.isFinite(Number(offsetMinutes))
    ? Number(offsetMinutes)
    : DEFAULT_CHANNEL_SCHEDULE_OFFSET_MINUTES;
  if (offset === 0) return 'UTC';
  const sign = offset > 0 ? '+' : '-';
  const abs = Math.abs(offset);
  const hours = Math.floor(abs / 60);
  const minutes = abs % 60;
  return `UTC${sign}${hours}${minutes ? `:${pad2(minutes)}` : ''}`;
}

// 解析 "2026-12-01" / "2026-12-01 09:30" / "2026-12-01T09:30:00" 这类墙上时间。
function parseWallClockString(value) {
  const matched =
    /^(\d{4})-(\d{1,2})-(\d{1,2})(?:[ T](\d{1,2}):(\d{1,2})(?::(\d{1,2}))?)?$/.exec(
      String(value ?? '').trim(),
    );
  if (!matched) return null;
  return {
    year: Number(matched[1]),
    month: Number(matched[2]),
    day: Number(matched[3]),
    hour: Number(matched[4] ?? 0),
    minute: Number(matched[5] ?? 0),
    second: Number(matched[6] ?? 0),
  };
}

// 墙上的年月日时分 -> 相对当前时区口径的 Unix 秒。
// 支持 Date（日期控件返回值，按控件显示的本地时间读数）与字符串两种输入。
// 无法解析时返回 NaN，由调用方提示格式错误。
export function wallClockToTimestamp(value, offsetMinutes) {
  let parts = null;
  if (value instanceof Date && !Number.isNaN(value.getTime())) {
    parts = {
      year: value.getFullYear(),
      month: value.getMonth() + 1,
      day: value.getDate(),
      hour: value.getHours(),
      minute: value.getMinutes(),
      second: value.getSeconds(),
    };
  } else {
    parts = parseWallClockString(value);
  }
  if (!parts) return NaN;

  const offset = Number.isFinite(Number(offsetMinutes))
    ? Number(offsetMinutes)
    : DEFAULT_CHANNEL_SCHEDULE_OFFSET_MINUTES;
  const utcMilliseconds = Date.UTC(
    parts.year,
    parts.month - 1,
    parts.day,
    parts.hour,
    parts.minute,
    parts.second,
  );
  return Math.floor(utcMilliseconds / 1000) - offset * 60;
}

// Unix 秒 -> 当前时区口径下的 "YYYY-MM-DD HH:mm"。
export function timestampToWallClock(timestampSeconds, offsetMinutes) {
  const timestamp = Number(timestampSeconds);
  if (!Number.isFinite(timestamp) || timestamp <= 0) return '';
  const offset = Number.isFinite(Number(offsetMinutes))
    ? Number(offsetMinutes)
    : DEFAULT_CHANNEL_SCHEDULE_OFFSET_MINUTES;
  const shifted = new Date((timestamp + offset * 60) * 1000);
  return (
    `${shifted.getUTCFullYear()}-${pad2(shifted.getUTCMonth() + 1)}-${pad2(shifted.getUTCDate())}` +
    ` ${pad2(shifted.getUTCHours())}:${pad2(shifted.getUTCMinutes())}`
  );
}
