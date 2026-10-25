export function scheduleLabel(expression?: string):string {
  if (!expression)
    return '—';
  let parts = expression.trim().split(/\s+/);
  if(parts.length===7){const year=parts[6]!;return scheduleLabel(parts.slice(0,6).join(' '))+(year==='*'?'':`（年份 ${year}）`)}
  if (parts.length === 6) {
    const second = parts[0]!;
    if (parts.slice(1).every(v => v === '*')) {
      if (second === '*')
        return '每秒';
      if (/^\*\/\d+$/.test(second))
        return `每 ${Number(second.slice(2))} 秒`;
    }
    if(second!=='0')return expression;
    parts = parts.slice(1);
  }
  if (parts.length !== 5)
    return '自定义时间';
  const [minute, hour, day, month, week] = parts;
  if (day !== '*' || month !== '*' || week !== '*')
    return '按指定日期执行';
  if (minute === '*' && hour === '*')
    return '每分钟';
  if (/^\*\/\d+$/.test(minute!) && hour === '*')
    return `每 ${Number(minute!.slice(2))} 分钟`;
  if (/^\*\/\d+$/.test(hour!) && /^\d+$/.test(minute!))
    return `每 ${Number(hour!.slice(2))} 小时`;
  if (hour === '*' && /^\d+$/.test(minute!))
    return Number(minute) === 0 ? '每小时' : `每小时第 ${Number(minute)} 分钟`;
  if (/^\d+$/.test(hour!) && /^\d+$/.test(minute!))
    return `每天 ${hour!.padStart(2, '0')}:${minute!.padStart(2, '0')}`;
  return '自定义时间';
}
